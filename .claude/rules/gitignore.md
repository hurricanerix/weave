---
paths:
  - "**/.gitignore"
---

# Gitignore Rules for Weave

## Structure

This project uses **component-scoped .gitignore files**. Each component manages its own ignores.

| .gitignore Location | Scope | File Types |
|---------------------|-------|------------|
| `/.gitignore` | Go project (root) | `*.go`, `go.mod`, `go.sum`, `*.html`, `*.css`, `*.ttf` |
| `/compute/.gitignore` | C daemon | `*.c`, `*.cpp`, `*.h`, `*.cu`, `*.o`, `*.so` |
| `/electron/.gitignore` | Electron app | `*.js`, `*.json`, `*.html`, `*.css`, `node_modules/` |
| `/packaging/.gitignore` | Packaging scripts | `*.yml`, `*.desktop`, `*.sh`, `*.png` |

## Rules

### Never cross boundaries

If you're working on C code in `compute/`, add ignores to `compute/.gitignore`, NOT the root.

If you're working on Go code, add ignores to the root `.gitignore`.

### Before modifying any .gitignore

1. **Identify the component** - What language/directory are you working in?
2. **Use the correct file** - Match component to .gitignore per table above
3. **If unsure, ask** - Don't guess

### Examples

**Adding a C build artifact:**
```
# WRONG - Adding to root
echo "*.o" >> .gitignore

# RIGHT - Adding to compute
echo "*.o" >> compute/.gitignore
```

**Adding a Go binary:**
```
# RIGHT - Go artifacts go to root
# Add !bin/ or similar to root .gitignore
```

**Adding node_modules:**
```
# WRONG - Adding to root
echo "node_modules/" >> .gitignore

# RIGHT - Adding to electron
echo "**/node_modules/" >> electron/.gitignore
```

## Root .gitignore is special

The root `.gitignore` uses an **allowlist pattern** (ignore everything, then explicitly allow). This is different from typical .gitignore files.

- It starts with `*` (ignore everything)
- Then uses `!pattern` to allow specific files
- Modifying it incorrectly can hide files from git

**Do not add ignore patterns to root .gitignore.** It doesn't work that way. If you need to ignore something at the root level, you're probably working in the wrong scope.

## When in doubt

Ask: "Which component owns this file type?"

- `.go` files → root
- `.c`, `.h`, `.o` files → compute
- `.js`, `.json`, `node_modules` → electron
- `.yml`, `.desktop` → packaging
