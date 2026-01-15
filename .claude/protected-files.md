# Protected Files

Before modifying ANY file, check if it's protected:
- `README.md` DO NOT MODIFY (except during /review-docs sessions)
- `docs/**` DO NOT MODIFY (except during /review-docs sessions)
- `.claude/**` DO NOT MODIFY

If a task seems to require modifying these, STOP and respond:
> "This task would require modifying [filename], which is protected.
> Please clarify if this is intentional or if task needs adjustment."

## Governed Files

These files have specific rules - see the corresponding rule file:
- `**/.gitignore` - See `.claude/rules/gitignore.md` for which .gitignore to modify

## Intentional Exceptions

These items were intentionally added by the maintainer and should not be flagged in audits:

- `README.md`: The `:warning:` emoji is intentional to highlight the prototype status