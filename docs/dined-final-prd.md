# Dined Product Requirements Document

Draft date: May 10, 2026  
Product: Dined  
Tagline: "Proof that nobody actually agreed on dinner."

## 1. Product Summary

Dined is a private, family-first restaurant memory app for tracking where the family has eaten, who picked the restaurant, who attended, how people rated it, and what should be remembered for next time.

The core question Dined answers is:

> Have we eaten here before, who picked it, and did we like it?

Dined is not a Yelp replacement and is not a public review platform. It is a personal dining ledger for Daniel, Jen, Caleb, and Aiden, with fast logging, quick recall, public readonly browsing, and playful family stats.

## 2. Goals

### Primary Goals

1. Make it fast to log a restaurant visit at the restaurant or shortly after eating.
2. Let the family quickly check whether they have eaten somewhere before.
3. Track each dining visit chronologically.
4. Track who picked the restaurant.
5. Track optional per-person ratings in 0.5 increments.
6. Track basic restaurant metadata from Google Places.
7. Provide a public readonly view of all visits, similar to Dejaview.
8. Add fun stats and trophies without making the app feel mean.
9. Establish a strong authentic retro diner visual identity.

### Non-Goals

1. Dined is not a public review network.
2. Dined is not a broad friend-group planning app.
3. Dined is not a Yelp or Google Reviews replacement.
4. Dined should not require long written reviews.
5. Dined should not require separate accounts for every family member.
6. Dined should not support bulk import in v1.
7. Dined should not launch with map-first browsing.

## 3. Target Users

The first version is for one family:

- Daniel
- Jen
- Caleb
- Aiden

These people are participant profiles, not separate required user accounts. Any of them may appear as picker, participant, and rater.

## 4. Platform

Recommended first version: mobile-first PWA.

Reasoning:

- The app is mostly logging, searching, and browsing.
- It should work well from a phone while at a restaurant.
- It can be added to an iPhone Home Screen.
- It can use geolocation with permission.
- It avoids native app overhead while the product shape is still being refined.

Desktop views still matter for public readonly browsing and visual direction exploration, but the first implementation should prioritize mobile logging ergonomics.

## 5. Auth and Access

### Writable Access

- One API token grants user access.
- The API token should create an authenticated session.
- An authenticated session can:
  - create restaurants
  - create visits
  - edit existing restaurant selections
  - edit existing visits
  - manage tags
  - toggle chain status

### Destructive Actions

Deleting a restaurant or visit should require a simple confirmation modal.

### Public Readonly Access

Public readonly pages should be available without write access.

Readonly pages should expose:

- every visit
- visit notes
- restaurant name
- saved Google-backed restaurant details such as address, phone, and other stored metadata
- ratings
- tags
- picker
- price level
- chain badge when enabled

Readonly pages must never expose edit controls or mutation actions.

## 6. Data Model

### Restaurant

Fields:

- `id`
- `name`
- `address`
- `latitude`
- `longitude`
- `phone`
- `website`
- `google_place_id`
- `google_rating`
- `google_price_level`
- `category`
- `is_chain`
- `created_at`
- `updated_at`

Notes:

- Google Places is the only external source for v1 if possible.
- Google may populate the initial category, but the category must remain editable.
- Chain locations should stay distinct because ratings and experience can differ by location.
- Chain detection should be a manual toggle in v1.
- If `is_chain` is true, show a small `Chain` badge near the restaurant title.

### Dining Visit

Fields:

- `id`
- `restaurant_id`
- `visited_at`
- `picked_by_person_id`
- `price_level`
- `notes`
- `created_at`
- `updated_at`

Rules:

- A visit requires restaurant, date, picker, and at least one rating.
- Picker must be one of Daniel, Jen, Caleb, or Aiden.
- Price level is structured as 1-5 dollar signs.
- Wait time is not structured in v1; use tags such as `Long Wait`.

### Person

Fields:

- `id`
- `name`
- `avatar_color`
- `created_at`

Seed people:

- Daniel
- Jen
- Caleb
- Aiden

### Visit Participant Rating

Fields:

- `id`
- `visit_id`
- `person_id`
- `rating`
- `created_at`
- `updated_at`

Rules:

- Rating supports 0.5 increments.
- No individual person is required to rate every visit.
- A saved visit should have at least one rating.

### Tag

Fields:

- `id`
- `name`
- `created_at`
- `updated_at`

Rules:

- Tags are global once created.
- Custom tags must allow spaces.
- Tags are optional on each visit.

Starter tags:

- Would Return
- Long Wait
- Great Service
- Overpriced
- Kid Approved

### Visit Tag

Fields:

- `visit_id`
- `tag_id`

## 7. Core User Flows

### Flow 1: Log Visit From Nearby

1. User opens Dined.
2. App requests geolocation permission if needed.
3. App shows nearby restaurants in a list-first view.
4. User selects a restaurant.
5. If the restaurant is already known, Dined shows prior visit context.
6. User taps `Log a Dine`.
7. User selects:
   - date/time, default now
   - picker
   - optional participant ratings
   - price level
   - tags
   - optional note
8. User saves the visit.
9. Visit appears in chronological history and public readonly view.

### Flow 2: Search Before Eating

1. User opens Search.
2. Search prompt says: `Have we eaten here before?`
3. User searches restaurant name.
4. Dined prioritizes personal history above new Google results.
5. Visited restaurants show:
   - last visit date
   - number of visits
   - picker from latest visit
   - ratings
   - tags
   - chain badge if enabled
6. New restaurants can be added from Google Places results.

### Flow 3: Manual Restaurant Add

1. User taps Add Restaurant.
2. User searches Google Places.
3. User selects a restaurant.
4. Dined saves Google-backed metadata.
5. User can edit category and manually toggle chain status.
6. App opens the visit logging flow.

### Flow 4: Public Readonly Browse

1. Visitor opens public Dined readonly view.
2. Visitor can browse every visit.
3. Visitor can view notes, ratings, tags, picker, and restaurant details.
4. Visitor cannot edit, delete, create, or manage data.

### Flow 5: Trophy Case

1. User opens Trophy Case / Stats.
2. App shows family stats, rankings, and joke awards.
3. Visual style shifts from booth interior to jukebox / record-inspired treatment.
4. Awards should be funny and affectionate.

## 8. Information Architecture

Recommended primary navigation:

1. Dines
2. Nearby
3. Search
4. Trophy Case
5. Settings

Recommended home approach:

- Main page uses the booth interior concept.
- Top priority is fast logging and recent memory recall.
- Combine nearby quick-add with recent dines.

## 9. Key Screens

### Home / Main Page

Visual direction: Concept #2, pastel retro diner booth interior.

Purpose:

- Provide the main family dining ledger.
- Make logging a visit easy.
- Show recent dining history.
- Surface nearby restaurants.

Content:

- Dined logo/title
- Tagline: `Proof that nobody actually agreed on dinner.`
- Search prompt: `Have we eaten here before?`
- Nearby quick-add list
- Recent visit list
- Primary CTA: `Log a Dine`

Visual notes:

- Teal booths
- Pink wall stripe
- Checkerboard floor
- Wall menu strips for recent visits
- Arrow sign treatment for search or primary CTA
- Warm, playful diner atmosphere

### Nearby

Purpose:

- Fast restaurant selection while at or near a restaurant.

Content:

- Nearby restaurants from Google Places
- List-first layout
- Distance
- Google-backed details when available
- `Dined before` state for known restaurants
- `Chain` badge when manually toggled
- CTA: `Dined Here` or `Log a Dine`

### Search

Purpose:

- Answer whether the family has eaten at a restaurant before.

Content:

- Search input: `Have we eaten here before?`
- Visited results first
- New Google Places results after visited results
- Last visit summary
- Ratings
- Tags
- Picker
- Add/log action

### Log Visit

Purpose:

- Make visit entry fast enough to complete at the restaurant.

Fields:

- Restaurant
- Date/time, default now
- Picker
- Price level, 1-5 dollar signs
- Optional rating for Daniel
- Optional rating for Jen
- Optional rating for Caleb
- Optional rating for Aiden
- Tags
- Custom tag creation
- Note

Validation:

- Restaurant required.
- Visit date required.
- Picker required.
- At least one rating required.
- Individual person ratings are optional.

### Restaurant Detail

Purpose:

- Show all history for one restaurant location.

Content:

- Restaurant name
- Address
- Phone
- Website
- Category
- Chain badge if enabled
- Google-backed metadata
- Total visits
- Average rating
- Best rating
- Most recent visit
- Visit timeline
- CTA: `Log Another Dine`

### Public Readonly Visit History

Purpose:

- Let anyone view the family dining ledger without editing.

Content:

- All visits
- Restaurant details
- Notes
- Picker
- Ratings
- Tags
- Price level
- Chain badge

Restrictions:

- No edit actions.
- No delete actions.
- No token/session controls.

### Trophy Case

Visual direction: Concept #3, jukebox and records.

Purpose:

- Add delight through playful stats.
- Make Dined feel more personal than a basic CRUD app.

Content:

- Record-style award cards
- Jukebox-inspired score/stats modules
- Family rankings
- Restaurant rankings
- Funny awards

Suggested awards:

- Safe Bet
- Table Divided
- Perfect 10
- House Favorite
- The Regular
- Risky Pick
- Redemption Meal
- Never Let Them Pick Again

Tone rule:

- Awards can tease the picker, but should stay affectionate.

## 10. Visual Design Direction

Overall direction: authentic retro diner.

Main page:

- Use the Concept #2 booth interior direction.
- It should feel like stepping into a family diner booth.
- Avoid generic dashboard cards.
- Avoid comic book styling.

Trophy case:

- Use the Concept #3 jukebox and records direction.
- Records can represent awards, restaurants, scores, or yearly stats.
- This screen can be more playful than the main app.

Visual keywords:

- 1950s diner
- pastel diner booth
- teal vinyl
- pink wall band
- black-and-white checkerboard floor
- laminated menus
- arrow signs
- chrome trim
- jukebox records
- soda fountain
- ticket pad

Preferred colors:

- Cream / warm white
- Cherry red
- Teal
- Pink
- Black
- Chrome gray
- Mustard accents

Typography:

- Dined logo can use retro script or diner sign lettering.
- Headers can use bold mid-century display typography.
- Body text must stay readable and not over-stylized.

## 11. Google Places Integration

Use Google Places if possible for v1.

Likely needed capabilities:

- Nearby restaurant lookup
- Text search
- Place details
- Place ID storage
- Address, coordinates, phone, website, Google rating, and price level where available

Implementation guidance:

- Put Google API calls behind the backend.
- Store only the fields Dined needs.
- Use field masks to avoid requesting unnecessary data.
- Set strict quota limits and billing alerts.

Pricing note:

Google Maps Platform is pay-as-you-go, but the current pricing table lists monthly free usage caps. For a few requests per month, Dined should remain inside the free usage caps, though billing still needs to be enabled.

Reference:

- https://developers.google.com/maps/billing-and-pricing/pricing
- https://developers.google.com/maps/documentation/places/web-service/usage-and-billing

## 12. MVP Scope

### Must Have

- API token session login
- Public readonly visit history
- Seed people: Daniel, Jen, Caleb, Aiden
- Google Places restaurant search
- Nearby restaurant list
- Save restaurant metadata
- Editable category
- Manual chain toggle
- Chain badge
- Log dining visit
- Visit date/time
- Picker
- Optional per-person ratings with 0.5 increments
- At least one rating per visit
- Price level, 1-5 dollar signs
- Starter tags
- Global custom tags with spaces
- Notes
- Chronological visit history
- Search visited restaurants
- Restaurant detail page
- Delete confirmation modal

### Should Have

- Basic trophy case
- Basic stats
- Best picker
- Most visited restaurant
- Highest rated restaurant
- Biggest rating split
- Favorite category
- Public readonly filters
- `Dined before` badges in nearby/search results

### Could Have Later

- Map view
- Photos
- Receipt upload
- Native iOS app
- Push reminders
- Random restaurant picker
- Recommendation mode
- Import from location or spending history
- More advanced chain detection

## 13. Success Criteria

Dined MVP is successful if:

1. A visit can be logged in under one minute.
2. The family can quickly answer whether they have eaten somewhere before.
3. Public readonly browsing works without exposing edit controls.
4. Google Places lookup is useful without creating duplicate confusion.
5. The app clearly remembers who picked each restaurant.
6. The visual design feels like Dined, not a generic restaurant tracker.
7. Trophy case stats feel funny enough to revisit.

## 14. Risks and Considerations

- Google Places cost and quota should be controlled from the start.
- Location-based search may require graceful fallback if geolocation is denied.
- Chain handling can become messy; v1 should rely on distinct locations plus manual chain badge.
- Public readonly notes are acceptable, but the UI should make it clear they are public.
- If logging feels slow, tags and notes should become secondary instead of blocking save.
- If the diner theme hurts readability, preserve readability over decorative authenticity.

## 15. Final Resolved Decisions

- Audience is one family.
- There is one writable user/session model.
- API token creates a session.
- Public readonly exposes every visit and notes.
- Saved Google-backed restaurant details can be public readonly.
- Readonly cannot edit.
- Destructive edits use a simple confirmation modal.
- Google APIs only for external restaurant data in v1.
- Nearby is list-first.
- Map view is later.
- Chains remain distinct by location.
- Chain badge is controlled by manual toggle.
- Ratings support 0.5 increments.
- At least one rating is required per visit.
- Individual person ratings are optional.
- Tags are global.
- Custom tags allow spaces.
- Category is editable.
- Main page visual direction is Concept #2 booth interior.
- Trophy case visual direction is Concept #3 jukebox and records.
- Tagline is: "Proof that nobody actually agreed on dinner."
