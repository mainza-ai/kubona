from pathlib import Path

from kubona.memory import tiered_storage


def test_tiered_storage_is_explicitly_unserved():
    assert tiered_storage.UNSERVED is True
    assert "no served surface" in tiered_storage.UNSERVED_REASON


def test_no_served_module_imports_tiered_storage():
    root = Path(__file__).resolve().parents[1] / "kubona"
    offenders = []
    for path in root.rglob("*.py"):
        if path.name in {"tiered_storage.py", "console_api.py", "__init__.py"}:
            continue
        text = path.read_text(encoding="utf-8", errors="ignore")
        if "tiered_storage" in text:
            offenders.append(str(path.relative_to(root)))
    assert offenders == [], f"unserved tiered storage imported by: {offenders}"
