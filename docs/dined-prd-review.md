# Dined PRD Review

Source email: "Dined PRD for mockups", received May 10, 2026 at 5:46 PM ET.

## Quick Read

Dined has a clear wedge: private restaurant memory, not discovery or public reviews. The strongest product idea is the combination of fast visit logging, "who picked it", per-person ratings, and recall before deciding where to eat. That is specific enough to avoid becoming a generic restaurant tracker.

## What Is Strong

- The core question is memorable and useful: have we eaten here before, who picked it, and did we like it?
- The MVP is appropriately focused on logging, search, nearby, history, and lightweight stats.
- "Picked by" is a strong differentiator because it turns a simple dining log into something personal and fun.
- Per-person ratings are more useful than a single average score and should stay central.
- PWA-first is a reasonable first build because the app is mostly CRUD, search, and location-assisted logging.
- The visual direction has enough specificity to guide mockups: diner menu, receipt, chrome, red vinyl, teal, mustard, and checkerboard.

## Product Gaps To Resolve

- Identity model needs a decision early. Simple household profiles are much faster than accounts for every participant, but they affect sharing, sync, and privacy.
- Restaurant data source needs a first choice. Supporting both Google Places and Yelp in v1 increases integration and matching complexity.
- Duplicate restaurant handling will matter. Places data, manual entries, renamed restaurants, and multiple locations can create messy history.
- Offline or poor-signal logging should be considered because users may log from inside restaurants.
- Visit ownership is now clear: one family-owned dataset with one writable account/API token and a public readonly view.
- Search ranking should prioritize personal history over external search results.
- Stats need guardrails so joke awards feel affectionate and never make the app feel mean.
- Photos, receipts, and notes are tempting but could slow the core logging flow if added too early.

## MVP Recommendation

Start with a family-first private PWA:

1. One API token grants user access and creates an authenticated session.
2. The four people are Daniel, Jennifer, Caleb, and Aiden.
3. Google Places is the only external restaurant source for v1.
4. Public readonly view should be supported, similar to Dejaview, and should expose every visit.
5. The default experience should make nearby list-first logging fast.
6. Half-point ratings are allowed.
7. Price level is structured as 1-5 dollar signs.
8. Tags start with a small default set and support global custom names with spaces.
9. Photos, receipts, maps, and bulk import wait until after the logging loop feels fast.

## Resolved Decisions

- Audience: one family, not broad friend groups or formal shared spaces.
- Account model: one writable user; family can share the same API token if needed.
- Auth: API token creates a session, and that session can create visits and edit existing selections.
- Destructive edits: deleting a restaurant or visit should require a simple confirmation modal.
- Readonly: public readonly view exposes every visit, including notes and saved Google-backed restaurant details, but never allows editing.
- Picker: one picker per visit, chosen from the four family members.
- Non-person pickers: skip "Everyone", "Kids", and "Random" for now.
- Ratings: support 0.5 increments; no person is required to rate every visit, but a saved visit should have at least one rating.
- Would return: handle through tags for now instead of a separate boolean.
- Tags: use five defaults plus global custom tag creation. Starter defaults: Would Return, Long Wait, Great Service, Overpriced, Kid Approved.
- Cuisine/category: use an editable dropdown with basic restaurant types.
- Nearby: nearby API results only.
- Chains: keep locations distinct, with a manual chain toggle and a possible "Chain" badge near the restaurant title.
- Map: defer; start with list-first.
- Import: no old-memory bulk import.
- Visual direction: authentic retro diner.
- Tagline: "Proof that nobody actually agreed on dinner."

## Remaining Questions

No major MVP product questions remain open.
