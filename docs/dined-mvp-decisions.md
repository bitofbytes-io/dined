# Dined MVP Decisions

Updated from Daniel's answers on May 10, 2026.

## Product Shape

- Audience: one family only.
- People: Daniel, Jen, Caleb, Aiden.
- Account model: one writable user/admin.
- Auth model: Google OAuth with an app-enforced allowlist is used for writable access and should generate a session.
- Write access: an authenticated session can create visits and edit existing selections.
- Destructive edits: deleting a restaurant or visit should require a simple confirmation modal.
- Public access: readonly view should be publicly viewable, similar to Dejaview.
- Public readonly scope: expose every visit, including notes and saved Google-backed restaurant details such as address, phone, and other stored metadata.
- Public readonly restrictions: readonly pages must not allow editing.
- Platform: mobile-first PWA remains the likely first build, but desktop design direction is being explored first.
- Visual direction: authentic retro diner, not comic book style and not just modern app with diner accents.
- Brand language: Dined is the main name.
- Tagline: "Proof that nobody actually agreed on dinner."

## Restaurant Data

- External source: Google APIs only if possible.
- Nearby behavior: show nearby restaurants only.
- Map: nice later, but start list-first.
- Chains and multiple locations: keep locations distinct because rating and experience can differ by location.
- Chain display: use a manual toggle for now and show a small "Chain" badge near the restaurant title when enabled.
- Bulk import: no.

## Visit Logging

- Most likely logging moment: at the restaurant or shortly after eating.
- Required minimum useful log: restaurant, date, picker, and at least one rating.
- Picker: one of the four family members.
- Ratings: support 0.5 increments.
- Participant ratings: nobody is required to rate individually, but a saved visit should have at least one rating.
- Price level: structured 1-5 dollar-sign rating.
- Wait time: not structured; use a tag such as "Long Wait".
- Would return: do not add as a separate structured field yet. Cover this through tags.

## Tags

- Start with about five default tags.
- Allow custom tags.
- Custom tags must allow spaces.
- Starter tags:
  - Would Return
  - Long Wait
  - Great Service
  - Overpriced
  - Kid Approved
- Custom tags are global once created.

## Category Dropdown

Use a restaurant category/cuisine dropdown in v1. Starter options:

- American
- Mexican
- Italian
- Pizza
- Burgers
- Breakfast
- Chinese
- Japanese
- Thai
- Indian
- BBQ
- Seafood
- Dessert
- Coffee
- Other

Google can populate the starting category, but the category should remain editable.

## Clarified Open Item

The earlier "Everyone", "Kids", and "Random" picker idea meant non-person picker labels for cases where no single person chose the restaurant. For this family-only version, keep picker as one of Daniel, Jen, Caleb, or Aiden unless a real need appears later.

## Google Places Cost Note

Google Maps Platform pricing is pay-as-you-go, but the current core pricing table lists monthly free usage caps. As of the official pricing page last updated May 8, 2026:

- Places API Nearby Search Pro: 5,000 free billable events/month, then $32 per 1,000 events up to 100,000.
- Places API Text Search Pro: 5,000 free billable events/month, then $32 per 1,000 events up to 100,000.
- Places API Place Details Essentials: 10,000 free billable events/month, then $5 per 1,000 events up to 100,000.
- Places API Place Details Essentials (IDs Only): unlimited free usage.

For a few requests per month, Dined should stay inside the free usage caps, but billing still needs to be enabled and API keys should have quota limits.
