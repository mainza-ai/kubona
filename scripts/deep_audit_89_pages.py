import os, glob, re, json

with open("wiki_audit_raw.json") as f:
    pages = json.load(f)

# Codebase index for deep symbol lookup
go_files = glob.glob("platform/core/**/*.go", recursive=True)
py_files = glob.glob("platform/bridge/**/*.py", recursive=True)
ts_files = glob.glob("platform/console/src/**/*.ts", recursive=True) + glob.glob("platform/console/src/**/*.tsx", recursive=True)
rs_files = glob.glob("platform/crates/**/*.rs", recursive=True)
all_code = go_files + py_files + ts_files + rs_files

code_contents = {}
for cf in all_code:
    try:
        with open(cf) as f:
            code_contents[cf] = f.read()
    except Exception:
        pass

def search_symbols(keywords):
    matched = []
    for cf, text in code_contents.items():
        for kw in keywords:
            if re.search(r'\b' + re.escape(kw) + r'\b', text, re.IGNORECASE):
                matched.append((cf, kw))
    return matched

audit_results = []

for page in pages:
    basename = page["basename"]
    path = page["path"]
    title = page["title"]
    ptype = page["type"]
    status = page["status"]
    
    with open(path) as f:
        full_text = f.read()
        
    # Extract key concepts and keywords
    words = re.findall(r'\b[A-Za-z0-9_-]{4,}\b', basename)
    # add capitalized terms from title
    title_terms = re.findall(r'[A-Z][a-z0-9]+', title)
    search_terms = list(set([basename, basename.replace("-", "_")] + words + [t.lower() for t in title_terms if len(t) > 3]))
    
    hits = search_symbols(search_terms)
    hit_files = sorted(list(set([h[0] for h in hits])))
    
    audit_results.append({
        "basename": basename,
        "title": title,
        "path": path,
        "type": ptype,
        "status": status,
        "hit_files": hit_files,
        "full_text": full_text
    })

print(f"Deep scanned {len(audit_results)} pages.")
with open("deep_audit_interm.json", "w") as f:
    json.dump([{k: v for k, v in a.items() if k != "full_text"} for a in audit_results], f, indent=2)
