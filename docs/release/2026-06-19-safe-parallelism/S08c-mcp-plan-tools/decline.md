# Coach decline — S08c-mcp-plan-tools

Date: 2026-06-21T07:10:08Z

## Reason for declining the proposed ack

Pin 2 (sworn://baton/rules source): internal/prompt/baton/rules.md is created by S21-canonical-baton (T3). DEFER the sworn://baton/rules MCP resource as a Rule-2 deferral (why: source not yet built; tracking: S21; ack: Coach 2026-06-21) until S21 lands — do NOT add a hard T4->T3 dependency. Pin 6 (spec defect): the reachability artefact references create_release via MCP, which is not a registered tool — amend the spec's reachability section to reference a real registered planning tool (or drop it). Apply mechanical pins 1/3/4/5 inline: add RegisterResource()/RegisterPrompt() + resources/read & prompts/get to server dispatch; create internal/prompt/baton/ (vendor track-mode.md, decide VERSION.txt); resolve yaml.v3 via ADR or stdlib per [[project_dep_policy]]; add cmd/sworn/mcp.go to planned_files.

## What the Implementer should do

Revise design.md to address the points above. Commit the revised design.md (state stays at `design_review`) and halt. Captain will re-review the revised design; Coach will issue a fresh ack/decline against the new version.
