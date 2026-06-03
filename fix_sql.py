import os

base = r"\\wsl.localhost\Ubuntu-20.04\home\tylerhu\github_project\japanese-learning-app"

# Fix grammar_store.go
path = os.path.join(base, "internal", "data", "grammar_store.go")
with open(path, 'r') as f:
    content = f.read()
content = content.replace("datetime('now')`,\n", "datetime('now'))`,\n")
with open(path, 'w') as f:
    f.write(content)
print("grammar_store.go fixed")

# Also verify all stores have correct updated_at usage
stores = [
    "internal/data/word_store.go",
    "internal/data/grammar_store.go",
    "internal/data/speaking_store.go",
    "internal/data/writing_store.go",
    "internal/data/translation_store.go",
]
for s in stores:
    p = os.path.join(base, s)
    with open(p) as f:
        c = f.read()
    has_order = "ORDER BY updated_at DESC" in c
    has_insert = "updated_at" in c and "datetime('now')" in c
    print(f"{s}: ORDER_BY_updated_at={has_order}, INSERT_updated_at={has_insert}")
