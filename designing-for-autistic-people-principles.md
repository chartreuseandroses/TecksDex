# Design Principles for Autistic Users

*Distilled from Irina Rusakova, "Designing for autistic people — overview of existing research" (UX Collective, 2020).*

---

## Framing

Autism is a neurological variant, not a deficit. The relevant design consequence is that autistic people tend to register more sensory and cognitive information, and each piece of it lands harder and less predictably. Interfaces that are busy, ambiguous, or unpredictable therefore cost more to use — sometimes to the point of being unusable.

Two caveats worth holding onto:

- **Autistic people differ enormously from one another.** No single configuration serves everyone, which is why control and personalisation matter more than any specific default.
- **Much of the underlying research studied children**, often in educational software, and adult findings frequently diverge. Where the source studies contradict each other, that is flagged below rather than smoothed over.

The broader finding: designing well for autistic users mostly produces better design for everyone.

---

## 1. Make the interface predictable before you make it interesting

Stability and consistency reduce the processing cost of every interaction.

- Use one consistent layout pattern for each type of information; don't vary the structure page to page.
- Keep visual structure explicit — clear regions, obvious hierarchy, generous margins.
- Favour a plain, muted "skin" that stays out of the way of the content.

## 2. Reduce what's on screen, not what's available

Autistic users may excel at processing large amounts of *static* information, while struggling with rapidly changing input. Simplify the surface, not the substance.

- One primary call to action per screen.
- Cluster controls; surface only the commonly used ones and let the rest be reachable.
- Keep everything about a single subject on one page rather than fragmenting it across steps.
- Deep, browsable information (Wikipedia-style) is a feature, provided it's structured and self-paced.

## 3. Write literally

Inferred meaning — tone, sarcasm, idiom, metaphor — is unreliable to decode and shouldn't carry any load the user needs.

- Plain language throughout; no figurative phrasing in interface copy.
- Label actions by what they do: "Attach file", not "Click here".
- Don't ship icon-only controls except for near-universal ones like a back arrow. Pair icon with text.
- Non-native speakers benefit from exactly the same rules.

## 4. Format text for scanning and calm

- Single, left-aligned column.
- Clear separation between text areas and everything else.
- Clean, unambiguous typefaces; distinct sections; large margins.

## 5. Use imagery only when it carries information

This is where the research diverges. Studies with autistic children found visual cues improved comprehension and recommended pairing images with words. Participatory work with autistic adults found the opposite preference: well-structured text was favoured over illustration, infographic, or video.

The reconcilable principle:

- Include a visual when it conveys something text cannot — a route to a building, the face of a person the user will meet, a diagram of a real relationship.
- Don't add visuals for decoration or polish.
- Never place text over an image background.
- Where images are used, keep the graphics simple.

## 6. Choose soft colours and moderate contrast

Hyper-sensitivity to luminance means bright colours and harsh contrast can register as sensory overload rather than emphasis. One study found autistic boys tended to prefer green and brown and to avoid yellow — plausibly a response to its high luminance.

- Use muted, low-saturation palettes.
- Keep text-to-background contrast clearly legible without going to maximum-intensity extremes.
- Treat high-luminance colours (especially yellow) as accents to use sparingly, not as fills.

## 7. Make navigation explicit and orienting

- A single toolbar. Simplify and clarify rather than adding wayfinding.
- Label every page clearly.
- Show a progress indicator on any journey longer than one page.
- Large, clearly hit-able buttons with both icon and text.
- Consider image-based results where they suit the content; some users prefer them to a list of links.

## 8. Let motion earn its place

- Animation and transition are welcome when they serve the interaction — a hover state that reveals detail, for example.
- Cut motion that exists for visual effect alone.
- Avoid partial or non-parallel movement: side panels and filter drawers that slide independently of the page are disorienting.
- Don't require horizontal scrolling to see content.

## 9. Never interrupt

Roughly 40% of autistic people have an anxiety disorder. What reads as a minor annoyance to a neurotypical user can be genuinely distressing here.

- No automatic pop-ups, including newsletter and consent overlays layered on top of results.
- No animated banners, and no autoplaying media — sound especially.
- Anything intrusive should be user-initiated.

## 10. Remove time pressure

- Let users save a form and return to it.
- Make any timeout generous, warn before it expires, and allow extension.
- Never pace an interaction faster than the user sets it.

## 11. Hand over control through personalisation

Given how much variation exists between individuals, adjustability does what a fixed default cannot. Assistive reading tools built for autistic users, and the National Autistic Society's own vivid/calm toggle, both work this way.

Worth exposing in settings:

- Typeface, text size, line spacing
- Colour theme, including text and background colours
- Intensity of imagery and visual elements

Personalisation also doubles as a feedback channel — what people actually change tells you what your defaults got wrong.

---

## Quick checklist

| | |
|---|---|
| Layout | Consistent, structured, one main action per screen |
| Content | Complete on one page, self-paced, logically ordered |
| Text | Single left-aligned column, clear font, wide margins |
| Imagery | Informational only, never behind text |
| Language | Literal, labelled, no metaphor or icon-only controls |
| Colour | Soft and muted; watch luminance, not just contrast ratio |
| Navigation | One toolbar, labelled pages, progress indicators |
| Motion | Functional only; no autoplay, no pop-ups |
| Timing | Saveable forms, generous timeouts |
| Control | Font, spacing, colour, and visual-intensity settings |
