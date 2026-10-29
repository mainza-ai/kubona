"""Principal-bound tenant identity + explicit cross-tenant grants (Phase-2).

The gateway resolves the verified principal per call and injects a server-side
`caller` identity (sub + tenant claim) into tool args. Tools MUST prefer the
injected identity over any caller-supplied requester/tenant string:

- effective tenant = the principal's tenant when a caller identity is present;
- a caller-supplied tenant that differs from the principal's tenant consults
  the cross-tenant grant registry: a live grant (see GRANT_DECISION) from the
  REQUESTED tenant to the caller's tenant/principal covering the operation
  allows it; otherwise it REFUSES (fail-closed);
- without an injected identity (direct in-process / stdio calls, which are
  transport-trusted), the supplied tenant is strictly validated but honored.

Tenant syntax is the existing refuse-don't-strip rule (`session.tenant_slug`)
plus exactness: the input must already BE the canonical slug. ``" Default "``
refuses instead of collapsing onto ``"Default"`` — silent collapsing merged
distinct tenants (``"Default"`` vs ``"default"`` vs ``" default "``) into
adjacent namespaces.
"""

from __future__ import annotations

import hashlib
import sqlite3
import threading
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from kubona.config import state_root
from kubona.mcp.auth import AuthError
from kubona.session import strict_tenant_slug


class CrossTenantRefusal(AuthError):
    """A caller-supplied tenant differs from the principal's tenant claim and
    no live cross-tenant grant covers the request.

    Named (not a bare AuthError/ValueError) so operators and tests can match
    the fail-closed cross-tenant refusal precisely.
    """


GRANT_DECISION = (
    "Cross-tenant grant decision (Phase-2): cross-tenant access requires an "
    "explicit, audited grant {grant_id, source_tenant, target_tenant (lab "
    "slug or 'sub:<principal>'), scope allowlist (explicit tools/resources — "
    "never '*'), expiry, grantor admin signature over the canonical payload "
    "'grant|<source>|<target>|<scopes csv, sorted>|<expiry UTC seconds>', "
    "revoked flag}. The grantor signature MUST verify against the source "
    "tenant's current registered admin key (tenant-admin-signed only — a "
    "self-signed foreign grantor refuses); wildcard scopes refuse at ISSUE "
    "time. Signature + scope match + unexpired + unrevoked are re-checked "
    "live on EVERY use (nothing cached past TTL); revocation takes effect on "
    "the next check. No grant = no access (default deny preserved). "
    "Single-registry rule: the Go tenants SQLite DB is the durable system "
    "of record (HTTP issue/revoke); `attach_go_registry()` lets Python "
    "enforcement verify the same rows read-only with byte-identical "
    "payloads — no parallel registry may mint competing authority."
)

# The read scope covering tenant-record reads (mirrors the Go tenants
# package's ReadScope — same string, same meaning, byte-identical
# canonicalization so signatures verify across languages).
READ_SCOPE = "read"

# Principal-bound grantee prefix: "sub:<id>" targets one principal instead
# of a whole lab tenant (mirrors Go's GrantPrincipalPrefix).
PRINCIPAL_PREFIX = "sub:"

_EXPIRY_FORMAT = "%Y-%m-%dT%H:%M:%SZ"


def _parse_go_datetime(raw: Any) -> datetime | None:
    """Parse a Go SQLite DATETIME into an aware datetime (or None).

    mattn/go-sqlite3 stores time.Time as `2006-01-02 15:04:05.999999999-07:00`
    (nanosecond fractions, space separator, numeric offset). Python's
    fromisoformat takes the space + offset but only microsecond fractions,
    so nanoseconds are truncated to 6 digits first. Naive values are assumed
    UTC (Go stores UTC unless the connection forced a zone — fail-loud would
    be better, but a naive row that parses is vanishingly rare and the
    signature check still binds the exact instant). Unparseable → None
    (the row is unverifiable: deny, loudly in logs, silently in verdict).
    """
    import re

    if raw is None:
        return None
    if isinstance(raw, datetime):
        out = raw
    elif isinstance(raw, (int, float)):
        try:
            out = datetime.fromtimestamp(float(raw), tz=timezone.utc)
        except (OverflowError, OSError, ValueError):
            return None
    else:
        text = str(raw).strip().replace("Z", "+00:00")
        text = re.sub(r"(\.\d{6})\d+", r"\1", text)
        try:
            out = datetime.fromisoformat(text)
        except ValueError:
            return None
    if out.tzinfo is None:
        out = out.replace(tzinfo=timezone.utc)
    return out


def _utc_seconds(expiry: datetime) -> datetime:
    if expiry.tzinfo is None:
        expiry = expiry.replace(tzinfo=timezone.utc)
    return expiry.astimezone(timezone.utc).replace(microsecond=0)


def grant_canonical_payload(source_tenant: str, target_tenant: str,
                            scopes: tuple[str, ...] | list[str],
                            expiry: datetime) -> bytes:
    """Exact bytes the source admin signs (byte-identical to Go's
    GrantCanonicalPayload: same separators, sorted scopes, UTC seconds)."""
    exp = _utc_seconds(expiry).strftime(_EXPIRY_FORMAT)
    return (f"grant|{source_tenant}|{target_tenant}|{','.join(sorted(scopes))}|{exp}"
            ).encode()


def revoke_canonical_payload(grant_id: str, reason: str) -> bytes:
    """Exact bytes the source admin signs to revoke (byte-identical to Go's
    RevokeCanonicalPayload). Revocation is idempotent and replay-harmless."""
    return f"grant-revoke|{grant_id}|{reason}".encode()


def grant_id_for(canonical: bytes, signature: bytes) -> str:
    """Content-addressed grant id: hex(sha256(canonical || raw signature))."""
    return hashlib.sha256(canonical + signature).hexdigest()


def _valid_grant_scope(scope: str) -> None:
    if not scope:
        raise CrossTenantRefusal("empty grant scope: refusing")
    if "*" in scope:
        raise CrossTenantRefusal(
            "wildcard scope refused: grants carry an explicit allowlist, never *")
    for ch in scope:
        if not (ch.islower() or ch.isdigit() or ch in ":._-"):
            raise CrossTenantRefusal(
                f"grant scope {scope!r} is off-vocabulary (want [a-z0-9:._-]): refusing")


def _valid_grant_target(target: str) -> str:
    if not target:
        raise CrossTenantRefusal("empty grant target: refusing")
    if "|" in target or "*" in target:
        raise CrossTenantRefusal(
            f"grant target {target!r} carries metacharacters: refusing")
    if target.startswith(PRINCIPAL_PREFIX):
        sub = target[len(PRINCIPAL_PREFIX):]
        if not sub or any(c in sub for c in "| \t\n") or len(target) > 128:
            raise CrossTenantRefusal(
                f"grant principal target {target!r} is malformed: refusing")
        return target
    try:
        return strict_tenant_slug(target)
    except ValueError as e:
        raise CrossTenantRefusal(f"grant target invalid: refusing ({e})") from e


def _check_ed25519_pubkey(pubkey_hex: str) -> bytes:
    try:
        raw = bytes.fromhex(pubkey_hex.strip())
    except ValueError as e:
        raise CrossTenantRefusal(f"invalid grantor public key: refusing ({e})") from e
    if len(raw) != 32:
        raise CrossTenantRefusal("invalid grantor public key length: refusing")
    return raw


def _verify_sig(pubkey_hex: str, signature_hex: str, canonical: bytes) -> None:
    from cryptography.exceptions import InvalidSignature
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
    raw = _check_ed25519_pubkey(pubkey_hex)
    try:
        sig = bytes.fromhex(signature_hex.strip())
    except ValueError as e:
        raise CrossTenantRefusal(f"invalid grant signature format: refusing ({e})") from e
    if len(sig) != 64:
        raise CrossTenantRefusal("invalid grant signature length: refusing")
    try:
        Ed25519PublicKey.from_public_bytes(raw).verify(sig, canonical)
    except InvalidSignature as e:
        raise CrossTenantRefusal("grant signature verification failed: refusing") from e


@dataclass
class Grant:
    """One audited cross-tenant authorization (stored server-side)."""
    grant_id: str
    source_tenant: str
    target_tenant: str
    scopes: tuple[str, ...]
    expiry: datetime
    grantor_pubkey_hex: str
    signature_hex: str
    revoked: bool = False


class GrantStore:
    """Process-local grant registry (fail-closed by construction: a restart
    clears in-memory state, so no stale grant survives — every grant must be
    re-issued).

    The store holds source-tenant admin keys (the trust roots, mirroring the
    Go tenants table) plus issued grants. All checks are live: signature vs
    the CURRENT source admin key, scope match, expiry, revocation flag.

    SINGLE REGISTRY RULE (Phase-2): the Go tenants SQLite DB
    (`~/.kubona/kubona.db`, table `tenant_grants`) is the durable system of
    record for grants issued over HTTP. `attach_go_registry()` points this
    store at that file (read-only — writers stay on the Go side) so Python
    enforcement sees exactly the grants Go issued, verified with the same
    byte-identical canonical payloads. Without an attachment the store is
    memory-only (tests, operator-driven flows); a restart then clears it.
    """

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._admins: dict[str, str] = {}
        self._grants: dict[str, Grant] = {}
        self._go_db: Path | None = None

    def attach_go_registry(self, db_path: str | Path | None = None) -> Path:
        """Point this store at the Go tenants SQLite DB (read-only lookaside).

        `db_path` defaults to `<state_root>/kubona.db` — the same default
        the Go daemon uses (`--db`, default `~/.kubona/kubona.db`), so local
        and single-host deployments need no configuration. Set KUBONA_STATE_DIR
        (or pass an explicit path) when the DB lives elsewhere; in container
        topologies mount the state volume read-only on the Python side.

        The file is opened `mode=ro` on EVERY check (never cached, never
        written): writers stay on the Go side, readers can never lock or
        corrupt the ledger DB. A missing/unreadable DB denies (fail-closed)
        while attached — detach to return to memory-only behavior.
        """
        p = Path(db_path).expanduser() if db_path is not None else state_root() / "kubona.db"
        with self._lock:
            self._go_db = p
        return p

    def detach_go_registry(self) -> None:
        """Forget the Go DB attachment (memory-only behavior restored)."""
        with self._lock:
            self._go_db = None

    # -- trust roots --------------------------------------------------------

    def register_admin_key(self, tenant: str, pubkey_hex: str) -> None:
        """Bootstrap a source tenant's admin key (first write wins — rotation
        needs the current key's signature, like the Go SetAdminKey rule)."""
        tenant = strict_tenant_slug(tenant)
        _check_ed25519_pubkey(pubkey_hex)
        key = pubkey_hex.strip().lower()
        with self._lock:
            if tenant in self._admins:
                if self._admins[tenant] != key:
                    raise CrossTenantRefusal(
                        f"admin key already set for {tenant!r}: rotation requires "
                        "the current admin signature (use rotate_admin_key)")
                return
            self._admins[tenant] = key

    def rotate_admin_key(self, tenant: str, new_pubkey_hex: str,
                         signature_hex: str) -> None:
        """Rotate a source admin key, authorized by the CURRENT key's
        signature over 'adminkey-rotation|<tenant>|<new_hex>'."""
        tenant = strict_tenant_slug(tenant)
        _check_ed25519_pubkey(new_pubkey_hex)
        with self._lock:
            current = self._admins.get(tenant)
            if current is None:
                raise CrossTenantRefusal(
                    f"no admin key registered for {tenant!r}: refusing rotation")
        canonical = f"adminkey-rotation|{tenant}|{new_pubkey_hex.strip().lower()}".encode()
        _verify_sig(current, signature_hex, canonical)
        with self._lock:
            # Re-read under the lock: a concurrent rotation must not be
            # silently overwritten by a stale-signature holder.
            if self._admins.get(tenant) != current:
                raise CrossTenantRefusal(
                    f"admin key for {tenant!r} changed during rotation: refusing")
            self._admins[tenant] = new_pubkey_hex.strip().lower()

    # -- lifecycle ----------------------------------------------------------

    def issue_grant(self, source_tenant: str, target_tenant: str,
                    scopes: tuple[str, ...] | list[str], expiry: datetime,
                    grantor_pubkey_hex: str, signature_hex: str,
                    now: datetime | None = None) -> Grant:
        """Record a grant authorized by the source admin's signature over the
        canonical payload. Tenant-admin-signed only: the signature must verify
        against the source tenant's CURRENT registered admin key — a
        self-signed foreign grantor refuses. Wildcards refuse at issue."""
        source = strict_tenant_slug(source_tenant)
        target = _valid_grant_target(target_tenant)
        scopes = tuple(scopes)
        if not scopes:
            raise CrossTenantRefusal("at least one scope is required: refusing")
        for s in scopes:
            _valid_grant_scope(s)
        exp = _utc_seconds(expiry)
        now = now or datetime.now(timezone.utc)
        if now.tzinfo is None:
            now = now.replace(tzinfo=timezone.utc)
        if not now < exp:
            raise CrossTenantRefusal(
                "grant expiry must be in the future: refusing already-expired grant")
        with self._lock:
            current = self._admins.get(source)
            if current is None:
                raise CrossTenantRefusal(
                    f"unknown source tenant {source!r}: no registered admin key — refusing")
            if current != grantor_pubkey_hex.strip().lower():
                raise CrossTenantRefusal(
                    f"grantor is not the {source!r} admin: refusing "
                    "(foreign-grantor forgery)")
        canonical = grant_canonical_payload(source, target, scopes, exp)
        _verify_sig(current, signature_hex, canonical)
        sig_raw = bytes.fromhex(signature_hex.strip())
        gid = grant_id_for(canonical, sig_raw)
        grant = Grant(grant_id=gid, source_tenant=source,
                      target_tenant=target, scopes=tuple(sorted(scopes)),
                      expiry=exp, grantor_pubkey_hex=current,
                      signature_hex=signature_hex.strip().lower())
        with self._lock:
            # Revocation is sticky: re-presenting the identical blob after
            # revoke does NOT revive it (otherwise anyone holding the public
            # blob could undo the owner's revocation). A fresh lifecycle
            # needs fresh bytes (e.g. a new expiry).
            existing = self._grants.get(gid)
            if existing is not None and existing.revoked:
                return existing
            self._grants[gid] = grant
        return grant

    def revoke_grant(self, grant_id: str, reason: str, pubkey_hex: str,
                     signature_hex: str) -> Grant:
        """Flip the revoked flag, authorized by the SOURCE tenant's current
        admin key over the revoke payload. Only the issuing lab revokes;
        effective on the very next check (checks re-read the row live)."""
        with self._lock:
            grant = self._grants.get(grant_id)
            if grant is None:
                raise CrossTenantRefusal(f"unknown grant {grant_id!r}: refusing")
            current = self._admins.get(grant.source_tenant)
        if current is None or current != pubkey_hex.strip().lower():
            raise CrossTenantRefusal(
                "revoker is not the issuing lab's admin: refusing")
        _verify_sig(current, signature_hex,
                    revoke_canonical_payload(grant_id, reason))
        with self._lock:
            grant = self._grants.get(grant_id)
            if grant is None:  # pragma: no cover — defensive
                raise CrossTenantRefusal(f"unknown grant {grant_id!r}: refusing")
            grant.revoked = True
            return grant

    def get_grant(self, grant_id: str) -> Grant | None:
        with self._lock:
            return self._grants.get(grant_id)

    def check_grant(self, source_tenant: str, grantee_tenant: str,
                    scope: str, grantee_sub: str = "",
                    now: datetime | None = None) -> Grant | None:
        """Live enforcement decision: the covering grant, or None. Every
        check re-reads the row, expiry, revocation flag, and re-verifies the
        signature against the CURRENT source admin key — never cached.

        Sources are unioned: a live verified grant in EITHER the in-memory
        store OR the attached Go registry allows (each side fully verified;
        a union of allows is sound because neither side can mint the
        other's authority)."""
        hit = self._check_memory_grant(source_tenant, grantee_tenant, scope,
                                       grantee_sub, now)
        if hit is not None:
            return hit
        return self._check_go_grant(source_tenant, grantee_tenant, scope,
                                    grantee_sub, now)

    def _check_memory_grant(self, source_tenant: str, grantee_tenant: str,
                            scope: str, grantee_sub: str = "",
                            now: datetime | None = None) -> Grant | None:
        """The in-memory registry check (factored out of check_grant)."""
        now = now or datetime.now(timezone.utc)
        if now.tzinfo is None:
            now = now.replace(tzinfo=timezone.utc)
        with self._lock:
            current = self._admins.get(source_tenant)
            grants = [g for g in self._grants.values()
                      if g.source_tenant == source_tenant and not g.revoked]
        if current is None:
            return None
        for g in grants:
            if g.target_tenant == grantee_tenant:
                pass
            elif grantee_sub and g.target_tenant == PRINCIPAL_PREFIX + grantee_sub:
                pass
            else:
                continue
            if scope not in g.scopes:
                continue
            if not now < g.expiry:
                continue
            canonical = grant_canonical_payload(
                g.source_tenant, g.target_tenant, g.scopes, g.expiry)
            try:
                _verify_sig(current, g.signature_hex, canonical)
            except CrossTenantRefusal:
                continue
            return g
        return None

    def _check_go_grant(self, source_tenant: str, grantee_tenant: str,
                        scope: str, grantee_sub: str = "",
                        now: datetime | None = None) -> Grant | None:
        """The attached Go registry check (read-only lookaside, no cache).

        Reads the source admin key + live (unrevoked) grant rows from the Go
        tenants SQLite DB and verifies with the same byte-identical canonical
        payloads the Go side signs. Any failure — missing file, bad schema,
        unknown tenant, scope/expiry/signature mismatch, revoked row — denies
        (returns None). Fail-closed, never cached, never written.
        """
        with self._lock:
            db = self._go_db
        if db is None:
            return None
        now = now or datetime.now(timezone.utc)
        if now.tzinfo is None:
            now = now.replace(tzinfo=timezone.utc)
        try:
            con = sqlite3.connect(f"file:{db}?mode=ro", uri=True, timeout=5.0)
        except (sqlite3.Error, OSError, ValueError):
            return None
        try:
            cur = con.execute(
                "SELECT admin_pubkey_hex FROM tenants WHERE id = ?", (source_tenant,))
            row = cur.fetchone()
            if row is None or not (row[0] or "").strip():
                return None
            current = row[0].strip().lower()
            cur = con.execute(
                "SELECT grant_id, target_tenant, scopes_csv, expiry, "
                "grantor_pubkey_hex, signature_hex FROM tenant_grants "
                "WHERE source_tenant = ? AND (revoked IS NULL OR revoked = 0)",
                (source_tenant,))
            rows = cur.fetchall()
        except (sqlite3.Error, OSError, ValueError):
            return None
        finally:
            try:
                con.close()
            except (sqlite3.Error, OSError):
                pass
        for grant_id, target, scopes_csv, expiry_raw, pubkey_hex, sig_hex in rows:
            if (pubkey_hex or "").strip().lower() != current:
                continue  # not the CURRENT admin key (rotated): dead row
            if target == grantee_tenant:
                pass
            elif grantee_sub and target == PRINCIPAL_PREFIX + grantee_sub:
                pass
            else:
                continue
            try:
                scopes = tuple(sorted(s.strip() for s in str(scopes_csv).split(",") if s.strip()))
                for s in scopes:
                    _valid_grant_scope(s)
            except (ValueError, AttributeError):
                continue
            if scope not in scopes:
                continue
            exp = _parse_go_datetime(expiry_raw)
            if exp is None or not now < exp:
                continue
            canonical = grant_canonical_payload(source_tenant, target, scopes, exp)
            try:
                _verify_sig(current, sig_hex, canonical)
            except CrossTenantRefusal:
                continue
            return Grant(grant_id=grant_id, source_tenant=source_tenant,
                         target_tenant=target, scopes=scopes, expiry=exp,
                         grantor_pubkey_hex=current,
                         signature_hex=(sig_hex or "").strip().lower())
        return None

    def grant_allows(self, source_tenant: str, grantee_tenant: str,
                     scope: str, grantee_sub: str = "",
                     now: datetime | None = None) -> bool:
        """Allow/deny for one (source, grantee, scope) triple. Default deny:
        unknown/revoked/expired/out-of-scope/forged grants all deny."""
        try:
            strict_tenant_slug(source_tenant)
        except ValueError:
            return False
        return self.check_grant(source_tenant, grantee_tenant, scope,
                                grantee_sub, now) is not None

    def reset(self) -> None:
        """Clear grants AND admin keys AND the Go attachment (test isolation).
        Production never calls this — a process restart is the only reset
        it gets."""
        with self._lock:
            self._admins.clear()
            self._grants.clear()
            self._go_db = None


# Process-shared registry consulted on the gateway path (fail-closed default:
# empty until grants are issued; a restart clears it).
_DEFAULT_STORE = GrantStore()


def get_grant_store() -> GrantStore:
    """The process-shared grant registry (what resolve_tenant consults)."""
    return _DEFAULT_STORE


def reset_grants() -> None:
    """Clear the process-shared registry (tests only)."""
    _DEFAULT_STORE.reset()


def principal_tenant(principal: Any) -> str:
    """Derive the tenant claim from a verified principal (fail-closed).

    Issued by `OAuth2TokenIssuer.issue(tenant=...)` and attached at decode;
    principals without a claim (dev provider, legacy test doubles) resolve to
    the `"default"` tenant. A present-but-malformed claim refuses rather than
    falling back.
    """
    raw = getattr(principal, "tenant", None) or "default"
    try:
        return strict_tenant_slug(raw)
    except ValueError as e:
        raise AuthError(f"principal carries an invalid tenant claim: refusing ({e})") from e


def caller_from_principal(principal: Any) -> dict[str, str]:
    """Server-side caller identity the gateway injects into tool args."""
    return {"sub": principal.sub, "tenant": principal_tenant(principal)}


def resolve_tenant(*, supplied: str | None, caller: dict[str, str] | None,
                   default: str = "default", scope: str = READ_SCOPE,
                   store: GrantStore | None = None) -> str:
    """Effective tenant for a tool call (injected identity wins, fail-closed).

    - With an injected `caller`: the principal's tenant governs. An omitted
      supply binds to it; a supply that differs consults the cross-tenant
      grant registry — a live grant from the REQUESTED tenant to the caller's
      tenant/principal covering `scope` allows it, otherwise it refuses with
      CrossTenantRefusal (GRANT_DECISION — no grant = no access).
    - Without one (direct in-process / stdio callers): the supply is strictly
      validated, or the legacy default applies when omitted.
    """
    if caller is not None:
        tenant = caller["tenant"]
        if supplied is not None and supplied != tenant:
            registry = store or _DEFAULT_STORE
            sub = caller.get("sub", "")
            if registry.grant_allows(supplied, tenant, scope, sub):
                try:
                    return strict_tenant_slug(supplied)
                except ValueError as e:
                    raise CrossTenantRefusal(
                        f"granted tenant {supplied!r} is malformed: refusing ({e})"
                    ) from e
            raise CrossTenantRefusal(
                f"caller-supplied tenant {supplied!r} differs from the principal's "
                f"tenant {tenant!r} and no live cross-tenant grant covers it: "
                f"refusing ({GRANT_DECISION})"
            )
        return tenant
    if supplied is None:
        return default
    return strict_tenant_slug(supplied)
