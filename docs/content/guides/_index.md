---
title: "Guides"
linkTitle: "Guides"
description: "Task-oriented walkthroughs for tomo-labs: how a run works end to end, reading the results table, the scenario suite, adding a new tool, and keeping the wired agents current."
weight: 15
featured: true
---

Each guide covers one job you actually do with tomo-labs, grounded in the real commands and files.
They assume you have worked through the [quick start](/getting-started/quick-start/) and have at least one tool built and run.
They are ordered the way you tend to reach for them: understand the machine, read what it produced, look at what it tested, then extend and maintain it.

- [How it works](/guides/how-it-works/) walks a run end to end: the proxy, the worker pool, and what keeps a rerun meaning the same thing. Read this first if you want to trust the numbers before you read them.
- [Results](/guides/results/) is the current comparison table across all eight wired tools, and the `00-hello` baseline that isolates each tool's fixed round-trip cost.
- [Scenarios](/guides/scenarios/) lists every task in the suite, from a baseline greeting through a small project scaffold.
- [Adding a tool](/guides/adding-a-tool/) covers the two files a new agent needs to join the comparison.
- [Upgrading the tools](/guides/upgrading-tools/) covers keeping the wired agents current as they ship new versions, and refreshing the captured results and prompts after a bump.

Beyond the core scenarios, the [evals](/evals/) section covers the eval tiers: whole public benchmarks rendered into the same task shape and selected with `--suite`.
