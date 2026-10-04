import json, re

with open("deep_audit_interm.json") as f:
    pages = json.load(f)

# Define audit categories and deep evaluation rules
def evaluate_page(p):
    base = p["basename"]
    hits = p["hit_files"]
    
    # Classify reality based on hits and code inspection
    # We will refine each specific page
    return {
        "basename": base,
        "title": p["title"],
        "status": p["status"],
        "type": p["type"],
        "hits_count": len(hits),
        "hit_files": hits
    }

evals = [evaluate_page(p) for p in pages]
print(f"Evaluated {len(evals)} pages.")
