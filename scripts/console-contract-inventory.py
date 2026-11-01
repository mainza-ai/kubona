#!/usr/bin/env python3
"""Generate the console response-contract inventory.

This is not OpenAPI and does not claim every route has a schema. It records:
  - every console API path,
  - every exported API function and the path/response contract it uses,
  - the contract names declared in responseContract.ts,
  - functions with no contract (the explicit backlog).

CI runs --check so this artifact cannot silently drift. Adding a contract or
changing a path requires regenerating it.
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
API = ROOT / "platform" / "console" / "src" / "lib" / "api.ts"
CONTRACT = ROOT / "platform" / "console" / "src" / "lib" / "responseContract.ts"
OUT = ROOT / "platform" / "contracts" / "console-contract-inventory.json"
GENERATED = ROOT / "platform" / "console" / "src" / "lib" / "generated" / "openapi.ts"

FUNC_RE = re.compile(r"export\s+(?:async\s+)?function\s+(\w+)\s*\(")
NEXT_EXPORT_RE = re.compile(r"\nexport\s+(?:async\s+)?(?:function|const|interface|type|class)\s+")
# Functions that do not expose a backend response shape at the console
# boundary: transport/browser/auth helpers, boolean status wrappers, and
# client-constructed envelopes. They are intentionally excluded from the
# response-schema backlog; their raw endpoint, if any, is covered by the path
# contract or has no JSON body.
CLIENT_ONLY = {
    "apiFetch": "transport primitive returning Response",
    "configureApiBase": "runtime base-URL probe",
    "downloadFile": "browser file helper",
    "isTerminalJobStatus": "status label helper",
    "jobStatusLabel": "status label helper",
    "lensImageProxyUrl": "same-origin URL helper",
    "reportError": "console error reporter",
    "resetDedupForTests": "test reset helper",
    "wasRecentAuthFailure": "auth timing helper",
    "authLogout": "boolean HTTP result",
    "authRevoke": "client-constructed ok envelope",
    "deleteSession": "boolean HTTP result",
    "loopAction": "client-constructed ok/error envelope",
    "exportLedger": "client-constructed export envelope",
    "exportSession": "client-constructed export envelope",
    "importSession": "client-constructed import envelope",
    "issueTenantGrant": "client-constructed grant envelope",
    "revokeTenantGrant": "client-constructed ok envelope",
    "runQuantumCanary": "client-constructed canary envelope",
    "scoreCodesignBudget": "client-constructed ok/result envelope",
    "fetchBackendStatus": "client-constructed health status",
}
NEXT_EXPORT_RE = re.compile(r"\nexport\s+(?:async\s+)?(?:function|const|interface|type|class)\s+")
PATH_RE = re.compile(r"\$\{API_BASE\}(/api/[^`\"\s]*)")
QUOTED_PATH_RE = re.compile(r"['\"](/api/[^'\"]+)['\"]")
CONTRACT_RE = re.compile(r"\b(?:contract|reportResponseContract|validateResponse)\(\s*['\"]([^'\"]+)['\"]")
REQUEST_OP_RE = re.compile(r"requestOperation\(\s*['\"]([^'\"]+)['\"]")
HELPER_OP_RE = re.compile(r"postTenantMutation\(\s*['\"](\w+)['\"]")
GENERATED_OP_RE = re.compile(r"^\s{2}(\w+): \{ method: \"(\w+)\", path: \"([^\"]+)\", response: \"([^\"]+)\" \},", re.MULTILINE)
METHOD_RE = re.compile(r"\bmethod:\s*['\"]([A-Za-z]+)['\"]")
SCHEMA_RE = re.compile(r"^\s{2}(?:'([^']+)'|([A-Za-z0-9_.]+)):\s*obj\(", re.MULTILINE)


def generated_operations() -> dict[str, dict[str, str]]:
    if not GENERATED.exists():
        return {}
    return {
        name: {"method": method.upper(), "path": path, "contract": contract}
        for name, method, path, contract in GENERATED_OP_RE.findall(GENERATED.read_text(encoding="utf-8"))
    }


def normalize(path: str) -> str:
    path = re.sub(r"\{([^}]+)\}", r":\1", path)
    path = re.sub(r"\$\{encodeURIComponent\([^)]*\)\}", ":id", path)
    path = re.sub(r"\$\{[^}]*\}", ":id", path)
    path = re.sub(r"//+", "/", path)
    path = re.sub(r"(?<!/):id", "", path)
    return path.rstrip("/") if not path.endswith("/") else path


def api_functions() -> list[dict[str, object]]:
    text = API.read_text(encoding="utf-8")
    generated = generated_operations()
    matches = list(FUNC_RE.finditer(text))
    out: list[dict[str, object]] = []
    for i, match in enumerate(matches):
        start = match.end()
        end = matches[i + 1].start() if i + 1 < len(matches) else len(text)
        # Stop at the next top-level export even if the next function regex
        # missed a declaration form.
        nxt = NEXT_EXPORT_RE.search(text, start, end)
        body = text[start:nxt.start()] if nxt else text[start:end]
        path = None
        m = PATH_RE.search(body) or QUOTED_PATH_RE.search(body)
        if m:
            path = normalize(m.group(1).split("?")[0])
        contract_match = CONTRACT_RE.search(body)
        method_match = METHOD_RE.search(body)
        operation = REQUEST_OP_RE.search(body) or HELPER_OP_RE.search(body)
        generated_op = generated.get(operation.group(1)) if operation else None
        if generated_op:
            if path is None:
                path = normalize(generated_op["path"])
            method = generated_op["method"]
        else:
            method = method_match.group(1).upper() if method_match else ("GET" if path else None)
        out.append(
            {
                "function": match.group(1),
                "path": path,
                "method": method,
                "contract": contract_match.group(1) if contract_match else (generated_op["contract"] if generated_op else None),
            }
        )
    return out


def schema_names() -> list[str]:
    text = CONTRACT.read_text(encoding="utf-8")
    block = re.search(r"export const RESPONSE_SCHEMAS = \{(.*?)\n\} satisfies", text, re.DOTALL)
    if not block:
        raise SystemExit("console-contract-inventory: RESPONSE_SCHEMAS block not found")
    return sorted({a or b for a, b in SCHEMA_RE.findall(block.group(1))})


def build() -> dict[str, object]:
    funcs = api_functions()
    schemas = schema_names()
    contracts = sorted({f["contract"] for f in funcs if f["contract"]})
    unknown = sorted(set(contracts) - set(schemas))
    if unknown:
        raise SystemExit(f"console-contract-inventory: contracts without schemas: {', '.join(unknown)}")
    unused = sorted(set(schemas) - set(contracts))
    if unused:
        raise SystemExit(f"console-contract-inventory: schemas with no API caller: {', '.join(unused)}")
    modeled = sorted((f for f in funcs if f["contract"]), key=lambda f: str(f["function"]))
    client_only = sorted(
        ({"function": f["function"], "reason": CLIENT_ONLY[str(f["function"])]}
         for f in funcs if not f["contract"] and str(f["function"]) in CLIENT_ONLY),
        key=lambda f: f["function"],
    )
    unmodeled = sorted(
        (f for f in funcs if not f["contract"] and str(f["function"]) not in CLIENT_ONLY),
        key=lambda f: str(f["function"]),
    )
    return {
        "generated_by": "scripts/console-contract-inventory.py",
        "contracts": contracts,
        "schemas": schemas,
        "modeled_functions": modeled,
        "client_only_functions": client_only,
        "unmodeled_functions": unmodeled,
    }


def main() -> int:
    inventory = build()
    rendered = json.dumps(inventory, indent=2, sort_keys=False) + "\n"
    if "--check" in sys.argv:
        if not OUT.exists():
            print(f"console-contract-inventory: missing {OUT.relative_to(ROOT)}", file=sys.stderr)
            return 1
        if OUT.read_text(encoding="utf-8") != rendered:
            print("console-contract-inventory: stale; run scripts/console-contract-inventory.py --write", file=sys.stderr)
            return 1
        print(
            f"console-contract-inventory: {len(inventory['modeled_functions'])} modeled, "
            f"{len(inventory['client_only_functions'])} client-only, "
            f"{len(inventory['unmodeled_functions'])} unmodeled, {len(inventory['schemas'])} schemas"
        )
        return 0
    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(rendered, encoding="utf-8")
    print(f"console-contract-inventory: wrote {OUT.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
