# Documentation standards for Weave

## Philosophy

Documentation is code. It should be:
- Precise (no ambiguity)
- Professional (no cutesy formatting)
- Scannable (easy to find information)
- Maintainable (easy to update)

## Writing style

### Tone
- Professional and direct
- Technical, not conversational
- Imperative for instructions: "Run this command", not "You should run this command"
- Active voice: "The daemon processes requests", not "Requests are processed"

### Clarity
- Short sentences - one idea per sentence
- Short paragraphs - 3-5 sentences max
- No marketing speak - "Fast GPU inference", not "Lightning-fast AI-powered magic"
- Concrete examples - show actual code/commands, not abstract concepts

## Formatting rules

### Use these elements

**Headers** - Organize content hierarchically
**Code blocks** - With language tags
**Lists** - For enumerations and steps
**Tables** - For comparisons and structured data
**Bold** - For emphasis (sparingly)
**Inline code** - For commands, filenames, variables

### Avoid these elements

**Emoji** - None. Ever. Not even in comments.
**Excessive bold/italic** - Makes text harder to scan
**ASCII art** - Except for architecture diagrams
**Exclamation marks** - Use sparingly, if at all
**Questions in headers** - "How does it work?" becomes "How it works"
**Marketing language** - "Amazing", "incredible", "game-changing"

## Emoji policy

No emoji in any documentation. Period.

Bad:
```markdown
## Quick start
- Fast performance
- Avoid this pattern
```

Good:
```markdown
## Quick start
- Fast performance
- Avoid this pattern
```

**Why no emoji:**
- Unprofessional in technical documentation
- Accessibility issues (screen readers)
- Inconsistent rendering across systems
- Adds noise, reduces clarity
- Ages poorly

**Exception:** None. If you think you need an emoji, use words instead.

## Code examples

### Always include language tags

Bad:
```
make test
```

Good:
```bash
make test
```

### Show complete context

Bad:
```go
Generate(prompt)
```

Good:
```go
img, err := client.Generate(ctx, prompt)
if err != nil {
    return fmt.Errorf("generation failed: %w", err)
}
```

### Include expected output

```bash
$ weave "a cat in space"
Generating... (10s)
Saved to: output.png
```

## Structure standards

### README.md format
1. Project name and one-line description
2. What it does (problem it solves)
3. Quick start / installation
4. Basic usage examples
5. Links to detailed docs

### Technical documentation format
1. Purpose (what problem this solves)
2. Design decisions (why this approach)
3. Implementation details (how it works)
4. Examples (show actual usage)
5. Edge cases / limitations

### API documentation format
1. Function signature
2. Parameters (with types and constraints)
3. Return values (with types)
4. Errors (what can fail and why)
5. Example usage

## Headers

### Capitalization
Use sentence case, not title case.

Bad:
```markdown
## Getting Started With Weave
```

Good:
```markdown
## Getting started with Weave
```

**Exception:** Proper nouns remain capitalized:
```markdown
## Using CUDA with Weave
```

### Hierarchy
- `#` - Document title (once per file)
- `##` - Major sections
- `###` - Subsections
- `####` - Rare, only if necessary

Do not skip levels:
```markdown
# Title
## Section
#### Subsection  (Bad - skipped ###)
```

## Lists

### When to use lists
- Unordered - Items without sequence/priority
- Ordered - Steps, rankings, sequences

### Formatting

Parallel structure:
```markdown
Bad:
- Fast generation
- Uses GPU
- You can run it as a daemon

Good:
- Fast generation
- GPU acceleration
- Daemon mode available
```

Consistent punctuation:
```markdown
Bad:
- Item one.
- Item two
- Item three;

Good (short items):
- Item one
- Item two
- Item three

Good (long items):
- This is a complete sentence.
- This is also a complete sentence.
- Every item ends with a period.
```

## Code comments

### In documentation
Use code blocks with comments inside:

```c
// Generate image from prompt
int generate(const char *prompt) {
    // Validate input
    if (prompt == NULL) {
        return ERR_NULL_POINTER;
    }
    
    // Process request
    return process(prompt);
}
```

### Comment style
- Comments explain "why", code shows "what"
- Comments use full sentences
- Comments do not repeat the code

Bad:
```c
// Set x to 5
int x = 5;
```

Good:
```c
// Default timeout is 5 seconds per RFC-1234
int timeout_seconds = 5;
```

## Tables

Use for structured comparisons:

```markdown
| Component | Language | Purpose |
|-----------|----------|---------|
| weave | Go | CLI and orchestration |
| weave-compute | C | GPU inference daemon |
```

Keep tables simple:
- 3-5 columns max
- Short cell content
- Not for long paragraphs
- Not for code blocks

## Links

### Internal links
Use relative paths:
```markdown
See [Architecture](ARCHITECTURE.md) for details.
See [Go rules](.claude/rules/go.md) for coding standards.
```

### External links
Include context:
```markdown
Bad:
See [here](https://example.com) for more info.

Good:
See the [CUDA documentation](https://docs.nvidia.com/cuda/) for kernel optimization.
```

## Command examples

### Show full commands
```bash
# Bad (incomplete)
weave generate

# Good (complete with options)
weave generate "a cat in space" --output=cat.png --steps=28
```

### Include context

```bash
# Start the daemon
sudo systemctl start weave-compute

# Verify it's running
systemctl status weave-compute

# Generate an image
weave "a cat in space"
```

## File references

Use inline code for:
- Filenames: `README.md`
- Paths: `/usr/local/bin/weave`
- Directories: `.claude/rules/`
- Extensions: `.go`, `.c`, `.md`

## Examples section

Every non-trivial feature needs examples.

**Include:**
- Minimal working example
- Common use case
- Expected output
- Error case (if relevant)

**Example structure:**
```markdown
## Examples

### Basic generation
```bash
weave "a cat in space"
# Output: Saved to output.png
```

### Custom dimensions
```bash
weave "a cat in space" --width=1024 --height=1024
# Output: Saved to output.png (1024x1024)
```

### Error handling
```bash
weave ""
# Error: prompt cannot be empty
```
```

## Anti-patterns

### Avoid these patterns

- Walls of text without structure
- Vague descriptions ("it's fast", "it's good")
- Missing examples
- Unexplained acronyms on first use
- Broken links
- Outdated examples that do not work
- Instructions without expected results
- Marketing language

### Use these patterns instead

- Break into headed sections
- Specific metrics ("generates in 10s on RTX 4070")
- Working code examples
- "CUDA (Compute Unified Device Architecture)"
- Test links before committing
- Verify examples work
- Show expected output
- Technical descriptions

## Documentation checklist

Before marking documentation complete:

- [ ] No emoji anywhere
- [ ] Headers use sentence case
- [ ] Code blocks have language tags
- [ ] Examples actually work
- [ ] Links are not broken
- [ ] No marketing language
- [ ] Technical terms defined on first use
- [ ] Commands show expected output
- [ ] No excessive formatting (bold/italic)
- [ ] Professional tone throughout

## When to document

### Always document
- Public APIs
- Configuration options
- Installation steps
- Deployment procedures
- Breaking changes
- Security considerations

### Do not over-document
- Self-explanatory code
- Internal implementation details
- Temporary scaffolding
- Obvious variable names

## Updating documentation

When code changes:
1. Update relevant docs in same commit
2. Check for broken examples
3. Update version numbers if applicable
4. Test any commands shown in docs

Stale documentation is worse than no documentation.