---
version: alpha
name: iFixAi Editorial Trust
description: A warm, high-contrast trust-first landing system mixing newsroom urgency with playful AI confidence.
colors:
  primary: "#6366F1"
  secondary: "#1D1A15"
  tertiary: "#FFFDF8"
  neutral: "#F3E8D4"
  surface: "#FFFFFF"
  on-surface: "#1D1A15"
  error: "#D93B30"
  success: "#1C9C4A"
  warning: "#F4B400"
  muted: "#8A7D67"
  border: "#1D1A15"
  accent-soft: "#E9E3FF"
typography:
  headline-display:
    fontFamily: "Inter"
    fontSize: "59px"
    fontWeight: 800
    lineHeight: 1.02
    letterSpacing: "-0.04em"
  headline-lg:
    fontFamily: "Newsreader"
    fontSize: "43px"
    fontWeight: 500
    lineHeight: 1.18
    letterSpacing: "-0.01em"
  headline-md:
    fontFamily: "Inter"
    fontSize: "31px"
    fontWeight: 800
    lineHeight: 1.15
    letterSpacing: "-0.03em"
  headline-sm:
    fontFamily: "Inter"
    fontSize: "22px"
    fontWeight: 500
    lineHeight: 1.18
    letterSpacing: "0em"
  body-lg:
    fontFamily: "Inter"
    fontSize: "18px"
    fontWeight: 500
    lineHeight: 1.55
    letterSpacing: "0em"
  body-md:
    fontFamily: "Inter"
    fontSize: "16px"
    fontWeight: 500
    lineHeight: 1.5
    letterSpacing: "0em"
  body-mono:
    fontFamily: "IBM Plex Mono"
    fontSize: "16px"
    fontWeight: 500
    lineHeight: 1.5
    letterSpacing: "0.14em"
  label-lg:
    fontFamily: "Inter"
    fontSize: "15px"
    fontWeight: 600
    lineHeight: 1.1
    letterSpacing: "0em"
  label-md:
    fontFamily: "Inter"
    fontSize: "14px"
    fontWeight: 600
    lineHeight: 1.1
    letterSpacing: "0.02em"
  label-sm:
    fontFamily: "IBM Plex Mono"
    fontSize: "12px"
    fontWeight: 500
    lineHeight: 1.1
    letterSpacing: "0.14em"
  caption:
    fontFamily: "IBM Plex Mono"
    fontSize: "12px"
    fontWeight: 500
    lineHeight: 1.4
    letterSpacing: "0.12em"
  overline:
    fontFamily: "IBM Plex Mono"
    fontSize: "11px"
    fontWeight: 500
    lineHeight: 1.1
    letterSpacing: "0.16em"
rounded:
  none: "0px"
  sm: "4px"
  md: "10px"
  lg: "14px"
  xl: "18px"
  full: "9999px"
spacing:
  xs: "2px"
  sm: "10px"
  md: "18px"
  lg: "32px"
  xl: "88px"
  gutter: "24px"
  section: "96px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.surface}"
    typography: "{typography.label-lg}"
    rounded: "{rounded.md}"
    padding: "13px 26px"
    height: "54px"
  button-secondary:
    backgroundColor: "transparent"
    textColor: "{colors.on-surface}"
    typography: "{typography.label-lg}"
    rounded: "{rounded.md}"
    padding: "13px 26px"
    height: "54px"
  button-tertiary:
    backgroundColor: "{colors.tertiary}"
    textColor: "{colors.on-surface}"
    typography: "{typography.label-md}"
    rounded: "{rounded.sm}"
    padding: "9px 14px"
    height: "36px"
  card:
    backgroundColor: "#FFFDF8"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.sm}"
    padding: "9px 12px"
    size: "auto"
  input:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.sm}"
    padding: "10px 12px"
    height: "44px"
  chip:
    backgroundColor: "#F5EEE1"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.full}"
    padding: "6px 10px"
    height: "28px"
# iFixAi Editorial Trust

## Overview
The interface feels like a refined editorial landing page with a playful, AI-native edge. It balances seriousness and confidence through strong typography, a warm cream backdrop, and vivid violet accents that signal modern software without feeling cold. The tone is professional but not sterile, with dense information presented in a spacious, highly legible layout.

## Colors
- **Primary (#6366F1):** The signature violet accent used for the main CTA, active states, and highlighted product moments. It gives the brand its modern AI energy and is the strongest color signal in the interface.
- **Secondary (#1D1A15):** A deep ink black used for headlines, body text, strokes, and structural elements. It provides the crisp, editorial contrast that makes the page feel authoritative.
- **Tertiary (#FFFDF8):** A soft warm white used for cards and surfaces that need subtle separation from the beige page background. It preserves the handcrafted, printed feel.
- **Neutral (#F3E8D4):** The warm paper-like background tone that supports the dotted pattern and keeps the page from feeling too clinical.
- **Surface (#FFFFFF):** Clean white reserved for lighter controls and utility surfaces where maximum contrast is needed.
- **On-surface (#1D1A15):** Primary readable text color across content, labels, and UI chrome.
- **Error (#D93B30):** Used for destructive or risky states, especially alerts and status pills that need immediate attention.
- **Success (#1C9C4A):** Used for pass states, confirmations, and positive audit outcomes.
- **Warning (#F4B400):** A caution accent for badges and credibility markers where attention is needed without escalation.
- **Muted (#8A7D67):** A subdued warm gray-brown for supporting metadata and secondary copy.
- **Border (#1D1A15):** The bold outline color that defines the system’s graphic, stamped-in look.
- **Accent-soft (#E9E3FF):** A pale lavender support tone for selected regions and softer emphasis around the primary violet.

## Typography
The system mixes three distinct voices: Inter for modern interface clarity, Newsreader for editorial sophistication, and IBM Plex Mono for technical/credential-like messaging. Headlines rely on high weight and tight tracking, especially the large hero title, which uses a dramatic sans-serif display treatment. The serif headline level adds a more literary, trust-building tone, while mono styles are used for labels, captions, and machine-flavored cues; these often appear in uppercase with increased letter spacing to create a stamped, systemized feel.

## Layout
The page uses a wide, two-column hero composition with generous negative space and a fixed-max-width feel rather than a fully fluid, edge-to-edge layout. Large horizontal breathing room is paired with strong vertical rhythm: compact metadata clusters, then prominent heading blocks, then clearly separated CTAs and supporting proof. Spacing follows a simple scale anchored by 2px, 10px, 18px, 32px, and 88px, which creates a mix of tight interface details and dramatic section-level gaps.

## Elevation & Depth
Depth is created through bold dark outlines, layered shadows, and slightly inset or lifted cards rather than soft blur-heavy elevation. Primary buttons and cards use a double-shadow effect that feels physical and sticker-like, while the UI panel on the right reads as a framed artifact. The system is not flat; hierarchy comes from contrast, borders, and stacked surfaces more than from subtle tonal gradients.

## Shapes
The shape language is pragmatic and slightly playful. Cards and inputs favor small radii for a crisp, utility-first feel, while primary buttons are a bit more rounded to make the CTA feel friendlier and more clickable. Full-pill shapes are reserved for status chips and compact badges, reinforcing the contrast between control surfaces and lightweight indicators.

## Components
Buttons are the most expressive component in the system. `button-primary` is a violet filled CTA with bold text, strong border treatment, and a tactile shadow; it should be used for the main action only. `button-secondary` is minimalist and transparent, relying on text and border rhythm rather than fill. `button-tertiary` and chip-like actions should stay compact, with smaller padding and a calmer visual presence. Hover states should intensify contrast or shift to a slightly richer violet, but should never lose the dark structural outline.

Cards use the warm `#FFFDF8` surface with a dark border and minimal radius, keeping content containers readable and tactile. They should feel like clipped paper or interface panels, not floating glass. Inputs should follow the same border language as cards, with a clean white or warm-white fill, generous padding, and clear focus states that can reuse the primary violet as a ring or border cue.

Chips and pills are used for statuses such as AUDITING, HIGH, and OK. These should be compact, rounded, and color-coded: error for risk, success for good states, and a softer blue-violet or neutral treatment for neutral process states. Lists inside cards should remain highly structured, with simple dividers, compact iconography, and strong alignment rather than decorative nesting.

## Do's and Don'ts
- Do keep the palette warm and high-contrast, with violet as the only dominant accent.
- Do use Inter for interface clarity and reserve Newsreader for editorial or trust-building headlines.
- Do preserve the bold outline-and-shadow look on primary actions and key cards.
- Do use mono styling for machine, audit, and metadata cues so the system feels technical and credible.
- Don't introduce soft pastel gradients, glassmorphism, or low-contrast grays that weaken the brand voice.
- Don't round every component equally; keep cards crisp and buttons slightly friendlier.
- Don't overuse the primary violet outside of CTAs, active states, and focal highlights.
- Don't make the layout overly dense; the design depends on measured whitespace to feel trustworthy.