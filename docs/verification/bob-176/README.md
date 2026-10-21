# BOB-176 home CTA visual evidence

These eight screenshots compare the home CTA before and after BOB-176 at a
390 px phone viewport and a 1440 px desktop viewport, for both logged-out and
logged-in audiences. Before the fix, the phone layout hides the CTA. After the
fix, it shows **View Dines** when logged out and **Log a Dine** when logged in.
The desktop captures retain both existing audience-specific CTAs.

## Fixture and capture provenance

All captures use the synthetic, memory-backed fixture from
`internal/repository/memory.go` (`repository.NewMemoryStore`) in a local Chrome
browser test. The diner names, restaurants, addresses, ratings, and notes shown
are repository demo data. No private photos, production data, real OAuth
credentials, or personal account details are included.

The capture harness is
`internal/server/home_cta_browser_test.go`,
`TestHomeCTAFitsBothAudiencesAtMobileAndDesktopWidths`, using
`DINED_HOME_SCREENSHOT_DIR`. Its logged-in audience uses a local test session
created by `newAuthenticatedTestRouter`; these captures do not establish live,
authenticated production verification. Before captures record the pre-fix
layout; after captures record the BOB-176 layout. The PNG files below are
byte-for-byte copies of the original local capture artifacts, with their
original filenames. The source paths identify those capture artifacts and are
not required to view this evidence.

## Capture inventory

| Layout | Audience | PNG dimensions | Repository file | Original capture artifact |
| --- | --- | --- | --- | --- |
| Before | logged-out | 390 × 844 px | [before/logged-out-390.png](before/logged-out-390.png) | `/tmp/bob176-before/logged-out-390.png` |
| Before | logged-out | 1440 × 900 px | [before/logged-out-1440.png](before/logged-out-1440.png) | `/tmp/bob176-before/logged-out-1440.png` |
| Before | logged-in | 390 × 844 px | [before/logged-in-390.png](before/logged-in-390.png) | `/tmp/bob176-before/logged-in-390.png` |
| Before | logged-in | 1440 × 900 px | [before/logged-in-1440.png](before/logged-in-1440.png) | `/tmp/bob176-before/logged-in-1440.png` |
| After | logged-out | 390 × 844 px | [after/logged-out-390.png](after/logged-out-390.png) | `/tmp/bob176-after/logged-out-390.png` |
| After | logged-out | 1440 × 900 px | [after/logged-out-1440.png](after/logged-out-1440.png) | `/tmp/bob176-after/logged-out-1440.png` |
| After | logged-in | 390 × 844 px | [after/logged-in-390.png](after/logged-in-390.png) | `/tmp/bob176-after/logged-in-390.png` |
| After | logged-in | 1440 × 900 px | [after/logged-in-1440.png](after/logged-in-1440.png) | `/tmp/bob176-after/logged-in-1440.png` |

The logged-in desktop PNG is identical before and after, which preserves the
existing desktop appearance. These static images show layout and CTA visibility;
the browser regression test separately checks animation, keyboard focus,
reduced-motion handling, CTA destinations, and viewport overflow.
