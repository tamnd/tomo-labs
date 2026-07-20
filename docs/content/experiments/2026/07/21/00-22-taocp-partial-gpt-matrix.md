---
title: "Stop at 1.1 million tokens: the TAOCP evaluator cost more than generation"
linkTitle: "TAOCP partial GPT matrix"
description: "A deliberately stopped TAOCP solver experiment shows that two full proof audits consumed more tokens and list-equivalent cost than solution generation. The completed paired cases also show no quality gain from slow mode despite 7.57 times the generation tokens. This report preserves the partial result without presenting an interrupted run as a complete model ranking."
date: 2026-07-21T00:22:30+07:00
---

Reproducibility header: task=five TAOCP exercises at levels 05, 15, 25, 30, and 35; solver=`taocp` matrix runner based on `5007fbed78ad745ba360e44675a4c3de8521f4b9`; generation models=gpt-5.6-sol, gpt-5.6-terra, gpt-5.6-luna, gpt-5.5, gpt-5.4, and gpt-5.4-mini through the local bridge and trace proxy; evaluator=gpt-5.6-sol; run window=`2026-07-20T15:42:14Z` to `2026-07-20T17:22:30Z`; raw trace requests=133; successful usage records=118; rate-limited responses=13; artifact tree SHA-256=`d214c917510c56f9f0f3a882dc8fc77ff8113b40ca9694f5c256674c93f39d35`.

The matrix was stopped deliberately after 1,108,154 tokens.
Continuing would have produced a cleaner leaderboard, but the run had already answered the more important engineering question: the evaluation design was too expensive for broad screening.

This is a partial experiment report.
It is not a completed six-model ranking.
The runner was under active development on a dirty working tree, so the artifact checksum, model responses, normalized case files, and proxy traces are the authoritative record.
The result is useful for cost accounting and for the fully paired fast-versus-slow comparison, but it should not be cited as an exactly reproducible benchmark release.

## Experiment design

Each model received the same section context and one of five exercises:

| Exercise | Level | Focus |
|---|---:|---|
| 1.2.1.1 | 05 | direct induction |
| 1.2.1.2 | 15 | find a proof flaw |
| 1.2.1.8 | 25 | identity proof |
| 1.2.1.11 | 30 | symbolic derivation |
| 7.2.1.2.93 | 35 | algorithm and proof |

Fast mode generated one complete solution.
Slow mode generated a population of candidates, reviewed them, and selected a final solution.
Every completed solution then faced two separate gpt-5.6-sol checks: an independent truth review and an adversarial proof audit.
The evaluator required mathematical truth, completeness, self-containment, readability, and verifiability before marking a solution publishable.

The five independent reference derivations were also produced by gpt-5.6-sol before candidate evaluation.
This reduced direct anchoring on a candidate, but it added another expensive model pass to every exercise family.

## What finished before the stop

The planned matrix contained 35 cases.
Twenty-seven completed, one ended during evaluation, four recorded provider errors while the run was being stopped or rate limited, and three gpt-5.4-mini cases never started.
Provider errors in this interrupted run are infrastructure outcomes, not evidence that a model could not solve an exercise.

| Generation profile | Completed | Publishable | Mean audit score | Coverage note |
|---|---:|---:|---:|---|
| gpt-5.6-sol fast | 5/5 | 4/5 | 6.4/7 | level 35 rejected |
| gpt-5.6-sol slow | 4/5 | 4/4 | 7.0/7 | level 35 not completed |
| gpt-5.6-terra fast | 5/5 | 4/5 | 6.2/7 | level 35 rejected |
| gpt-5.6-luna fast | 5/5 | 4/5 | 6.2/7 | level 35 rejected |
| gpt-5.5 fast | 5/5 | 4/5 | 6.2/7 | level 35 rejected |
| gpt-5.4 fast | 3/5 | 3/3 | 7.0/7 | level 30 evaluation interrupted; level 35 provider error |
| gpt-5.4-mini fast | 0/5 | 0/0 | unavailable | two provider errors; three not started |

All four completed level-35 fast solutions were rejected by both final decision fields.
The proof auditor identified a different unsupported graph or linear-extension lemma in each answer.
That agreement is useful evidence that exercise 7.2.1.2.93 separates polished exposition from a verified proof, but it is still an evaluator result, not an independent formal proof of incorrectness.

## Slow mode spent more without changing the paired outcome

Only the first four gpt-5.6-sol exercises form a complete fast-versus-slow pair.
Both modes produced four publishable solutions with perfect 7/7 audit scores.

| Mode | Publishable | Generation tokens | Audit tokens | Total tokens | Generation cost | Audit cost | Combined cost |
|---|---:|---:|---:|---:|---:|---:|---:|
| fast | 4/4 | 27,481 | 61,388 | 88,869 | $0.336955 | $0.583690 | $0.920645 |
| slow | 4/4 | 208,000 | 59,621 | 267,621 | $2.657850 | $0.552830 | $3.210680 |

Slow mode used 7.57 times the generation tokens and 7.89 times the generation cost.
Including the common two-pass audit, it used 3.01 times the tokens and 3.49 times the cost.
There was no measured quality improvement on these four paired exercises.

The missing level-35 slow result matters.
Fast mode failed that exercise, while slow mode was stopped before producing an auditable answer.
The experiment therefore cannot determine whether slow mode helps on the hardest case.

## The evaluator became the majority of the campaign

The normalized case artifacts account for 436,419 generation tokens and 469,245 final-audit tokens.
The five stored reference derivations add 51,257 tokens.
Retries and successful responses that did not become final case artifacts account for the rest.

The proxy trace is the correct operational ledger because it includes completed responses later discarded by retries or interruption:

| Work class | Successful calls | Input tokens | Output tokens | Total tokens | List-equivalent cost |
|---|---:|---:|---:|---:|---:|
| solution authoring and slow selection | 46 | 252,017 | 208,904 | 460,921 | $6.135873 |
| independent references and two proof audits | 72 | 459,581 | 187,652 | 647,233 | $7.927465 |
| campaign total | 118 | 711,598 | 396,556 | 1,108,154 | $14.063338 |

Reasoning tokens are included in output and are not added again.
No cache-read tokens were reported for this run.
Costs use the observed model on each successful request and the [OpenAI standard token prices](https://developers.openai.com/api/docs/pricing) in effect when this report was written.

Evaluation consumed 58.4 percent of all tokens and 56.4 percent of the list-equivalent cost.
The two final auditors alone made 67 successful calls for 589,167 tokens.
The design asked for two flagship-quality reviews even when a cheap generator produced an obviously straightforward answer, so the verifier erased much of the price difference among generation models.

The 13 HTTP 429 responses carried no usage records and add no token cost, but they did add latency and contributed to incomplete coverage.

## Why the matrix stopped

The original evaluator was designed for the strongest possible publication gate, not for an economical model screen.
That distinction became visible only after running the full trace:

1. Every candidate paid for two long gpt-5.6-sol reviews regardless of exercise difficulty or first-review confidence.
2. Independent reference construction front-loaded another flagship call per exercise.
3. Structured decision retries could consume a valid mathematical review and then repeat work because the final fields were malformed or incomplete.
4. Slow generation multiplied authoring cost before reaching the same fixed audit gate.
5. Rate limits made unfinished cells more expensive operationally without improving the statistical comparison.

Stopping was the correct experimental decision once the marginal matrix cells were unlikely to change those findings.
The interruption is disclosed instead of converting partial coverage into a leaderboard.

## A cheaper evaluation design

The next matrix should use a staged gate:

1. Run deterministic checks first, including required sections, parseable formulas, executable examples, and problem-specific invariants where available.
2. Use one compact truth evaluator for every candidate with a strict structured response contract.
3. Escalate to a second independent auditor only on disagreement, a low confidence decision, the hardest exercise tier, or a prospective published winner.
4. Generate each independent reference once, version it, and reuse it across model runs.
5. Keep a fixed calibration subset under the full two-auditor protocol so the cheaper gate can be measured for false accepts and false rejects.
6. Compare modes only on paired completed exercises and stop a profile early when its confidence interval cannot change the decision.

This keeps strong review where it matters while preventing evaluator spend from dominating broad exploration.
The full two-auditor flow remains appropriate for final solutions intended for publication.

## Preserved evidence

The stopped run remains under `/Users/apple/data/taocp-gpt-matrix-2026-07-20` with 32 case artifacts and the generated report.
The proxy ledger remains under `/Users/apple/data/taocp-matrix-traces/gpt-proxy` with request, latency, and usage streams.
The combined proxy files occupy 2,796,951 bytes.

The matrix processes, proxy, and bridge were terminated after the stop request.
No additional model calls were made while preparing this report.

Metrics: 35 cases planned; 27 completed; 1 evaluation error; 4 provider errors; 3 not started; 118 successful model responses; 13 rate-limited responses; 1,108,154 total tokens; 282,847 reasoning tokens included in output; $14.063338 total list-equivalent cost; evaluator share 58.4 percent of tokens and 56.4 percent of cost; paired gpt-5.6-sol fast and slow quality 4/4 publishable in both modes.
