@CONTRIBUTING.md

## Code comments

The default is still no comments — CONTRIBUTING.md's "no explanatory comments" rule stands, and code should explain itself through clear naming and structure. This section is about the exceptions: the rare comment that is genuinely needed because the code alone can't carry the context (a surprising invariant, a workaround for an external bug, a non-obvious decision).

When you do write one of those comments, write it for a reader who is new to the codebase but familiar with the goal of the project. Avoid jargon-dense shorthand: explain *why* the code does what it does in plain language, spell out non-obvious context (invariants, gotchas, links to the decision), and don't assume the reader knows internal nicknames, prior incidents, or module history. A good test: someone on day one who knows what the product does should understand the comment without grepping elsewhere.
