@CONTRIBUTING.md

## Code comments

Comments are rare by default — CONTRIBUTING.md's "no explanatory comments" rule means don't narrate what the code plainly does. But when a comment is genuinely warranted (a non-obvious invariant, a gotcha, a link to the decision behind the code), write it for a reader who is new to the codebase but familiar with the goal of the project. Avoid jargon-dense shorthand: explain *why* the code does what it does in plain language, and don't assume the reader knows internal nicknames, prior incidents, or module history. A good test: someone on day one who knows what the product does should understand the comment without grepping elsewhere.
