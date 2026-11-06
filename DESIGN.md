# Snippit — Design System & UI Guidelines

**Product:** Snippit
**Document:** Design Specification
**Version:** 1.0
**Design Direction:** Modern Developer Tool / Dark UI

---

## 1. Design Philosophy

Snippit should feel like a tool developers genuinely enjoy using.

The interface should be inspired by modern developer products such as Raycast, Linear, Vercel, and contemporary code editors.

The visual experience must be:

- Minimal.
- Technical.
- Modern.
- Fast-looking.
- Comfortable for extended reading.
- Focused on the code, not decoration.

### Design Principle

**The editor is the product. Everything else is secondary.**

Avoid unnecessary cards, excessive gradients, oversized illustrations, and complicated navigation.

## 2. Brand Identity

### Brand Name

Snippit

### Tagline

Your code. One link away.

### Brand Personality

- Developer-first.
- Lightweight.
- Reliable.
- Straightforward.
- Modern.

### Logo Direction

A minimal code-related symbol.

Suggested concepts:

- A stylized `{ }` symbol.
- A combination of a document and code brackets.
- A minimal folded snippet icon.

The wordmark should use a clean sans-serif typeface.

## 3. Color System

Primary theme: Dark.

### Core Colors

| Token | Hex | Usage |
|---|---|---|
| Background | `#0D1117` | Main background |
| Surface | `#151B23` | Panels |
| Elevated | `#1C2430` | Menus and popovers |
| Border | `#2A3441` | Dividers and borders |
| Primary | `#65D9A4` | Primary accent |
| Primary Hover | `#4CC78E` | Hover states |
| Text Primary | `#E6EDF3` | Main text |
| Text Secondary | `#9BA8B7` | Secondary text |
| Muted | `#687586` | Metadata |
| Danger | `#F07178` | Errors |
| Warning | `#E6BD72` | Warnings |
| Success | `#65D9A4` | Success |

### Color Usage

The emerald accent should be used selectively:

- Primary action buttons.
- Active navigation indicators.
- Focus states.
- Selected controls.
- Success feedback.

Do not use the accent color for large decorative backgrounds.

## 4. Typography

Use a combination of sans-serif and monospace typography.

### Interface Font

Recommended: Inter or Geist Sans.

Used for:

- Navigation.
- Headings.
- Buttons.
- Metadata.
- Form labels.

### Code Font

Recommended: JetBrains Mono.

Used for:

- Code editor.
- Code viewer.
- Line numbers.
- Raw content.
- Technical identifiers.

### Typography Scale

| Element | Size | Weight |
|---|---:|---|
| Main heading | 32px | 650 |
| Section heading | 20px | 600 |
| Body | 14px | 400 |
| Secondary text | 13px | 400 |
| Metadata | 12px | 400 |
| Code | 13px | 400 |
| Button | 13px | 600 |

Use a comfortable line height for code, approximately 1.65.

## 5. Layout System

### General Layout

- Maximum content width: 1200px.
- Main editor width: 100%.
- Horizontal page padding: 24px.
- Desktop header height: 64px.
- Standard border radius: 8px.
- Component spacing: 8px increments.

The application should use a centered layout with sufficient breathing room.

### Navigation

Navigation should remain minimal.

Left:
- Snippit logo.

Right:
- Documentation (optional).
- GitHub (optional).
- Theme toggle (future).

No account menu, login button, or registration CTA.

## 6. Homepage Design

### Layout Structure

```text
+------------------------------------------------------+
|  [Snippit]                              Docs  GitHub  |
+------------------------------------------------------+
|                                                      |
|                 Your code. One link away.            |
|       A fast, simple place to share your snippets.   |
|                                                      |
|  +------------------------------------------------+  |
|  | Untitled paste                 [JavaScript  v] |  |
|  |                                                |  |
|  |  1  const hello = () => {                      |  |
|  |  2    console.log("Hello, world!");            |  |
|  |  3  };                                         |  |
|  |  4                                             |  |
|  +------------------------------------------------+  |
|                                                      |
|  Expiration: [7 Days v]                              |
|                                                      |
|  [ Create Paste ]                                    |
|                                                      |
|       No signup. No friction. Just paste.            |
|                                                      |
+------------------------------------------------------+
|                   Snippit                            |
|             Built for developers.                    |
+------------------------------------------------------+
```

### Homepage Components

#### A. Header

- Logo on the left.
- Documentation and GitHub links on the right.
- Transparent or background-matched surface.
- Thin bottom border.

#### B. Hero Section

Headline:

"Your code. One link away."

Supporting text:

"A fast, simple place to share your code snippets, logs, and configuration files."

Keep the hero compact. The editor should be visible without excessive scrolling on desktop.

#### C. Paste Editor

The main focus of the homepage.

Features:

- CodeMirror 6.
- Line numbers.
- Syntax highlighting.
- Current language indicator.
- Tab indentation.
- Automatic indentation.
- Code selection.
- Keyboard shortcuts.
- Dark editor theme.

The editor should have a subtle border and dark surface.

Recommended initial editor height: 380px.

The editor should expand naturally with content, with a sensible maximum height before internal scrolling.

#### D. Paste Settings

Place settings below the editor.

Fields:

- Optional title.
- Language selector.
- Expiration selector.

Keep them compact and visually secondary.

#### E. Create Button

Primary CTA:

`Create Paste →`

Style:

- Emerald background.
- Dark text.
- Medium font weight.
- 8px border radius.
- Clear hover and focus states.

While submitting, show a loading state and prevent duplicate submission.

## 7. Paste Viewer Design

The paste viewer should feel like a standalone code editor or source viewer.

### Layout Structure

```text
+------------------------------------------------------+
| [Snippit]                               Docs GitHub   |
+------------------------------------------------------+
|                                                      |
|  Example JavaScript                                  |
|  JavaScript · Created 5 minutes ago                  |
|                                                      |
|  [Copy] [Raw] [Download] [Wrap]                      |
|                                                      |
|  +------------------------------------------------+  |
|  |  1  const hello = () => {                       |  |
|  |  2    console.log("Hello, world!");             |  |
|  |  3  };                                          |  |
|  |  4                                              |  |
|  +------------------------------------------------+  |
|                                                      |
|  Paste ID: 7kP2mX9qLa                                |
|                                                      |
+------------------------------------------------------+
```

### Viewer Components

#### Paste Header

Display:

- Paste title.
- Language.
- Relative creation time.
- Expiration metadata when applicable.

#### Action Toolbar

Actions:

| Action | Behavior |
|---|---|
| Copy | Copies complete content |
| Raw | Opens raw text endpoint |
| Download | Downloads content |
| Wrap | Toggles line wrapping |

Use compact buttons with icons and labels.

#### Code Viewer

- Read-only.
- Syntax highlighting.
- Line numbers.
- Horizontal scrolling when wrapping is disabled.
- Clear contrast between code and line numbers.
- Optional line highlighting.

Do not allow pasted content to inject HTML or executable JavaScript into the page.

## 8. Component Design

### Buttons

Primary:
- Emerald background.
- Dark foreground.
- Medium weight.

Secondary:
- Dark surface.
- Subtle border.
- Light foreground.

Ghost:
- Transparent.
- Muted foreground.
- Surface hover state.

Danger:
- Red accent.
- Reserved for destructive operations.

### Inputs

- Dark surface.
- Subtle border.
- 8px radius.
- 14px typography.
- Visible focus ring.

### Dropdowns

Used for:

- Language selection.
- Expiration selection.

Requirements:

- Keyboard navigable.
- Clear selected state.
- Searchable language list.
- Compact menu presentation.

### Toast Notifications

Examples:

- `Paste created successfully`
- `Copied to clipboard`
- `Download started`
- `Something went wrong`

Toasts should be subtle and disappear automatically.

## 9. Editor Experience

CodeMirror 6 is the preferred editor.

### Supported Languages

Initial languages:

- Plain Text.
- JavaScript.
- TypeScript.
- HTML.
- CSS.
- JSON.
- XML.
- Python.
- PHP.
- Go.
- Rust.
- Java.
- C.
- C++.
- C#.
- Bash.
- SQL.
- YAML.
- Markdown.
- Dockerfile.
- Nginx.

Language selection should use a searchable dropdown.

Automatic language detection can be added as an enhancement, but should never block paste creation.

### Editor Behavior

- Default language: Plain Text or automatic detection.
- Tab inserts indentation.
- Enter maintains indentation.
- Line numbers enabled.
- Soft wrapping configurable.
- Editor content should remain accessible on mobile.

## 10. Responsive Design

### Desktop

- Centered content.
- Maximum width around 1200px.
- Editor uses most available horizontal space.
- Toolbar stays on one line.

### Tablet

- Reduced horizontal padding.
- Full-width editor.
- Compact toolbar.

### Mobile

- 16px horizontal padding.
- Compact header.
- Full-width editor.
- Smaller code font when needed.
- Toolbar can wrap or scroll horizontally.
- Primary CTA spans full width.

The editor must remain usable on narrow screens without forcing the entire page to overflow.

## 11. Interaction and Micro-Interactions

Keep interactions subtle.

- Button hover transitions.
- Focus rings on inputs.
- Smooth dropdown opening.
- Copy success feedback.
- Loading indicator during paste creation.
- Subtle toast transitions.

Avoid excessive animations.

Prefer transitions between 120ms and 180ms.

Respect reduced-motion preferences.

## 12. Accessibility

- Semantic HTML.
- Accessible form labels.
- Keyboard navigation.
- Visible focus states.
- Sufficient contrast.
- Screen-reader labels for icon-only buttons.
- Accessible toast announcements.
- Editor keyboard usability.

## 13. Empty and Error States

### Empty Editor

Placeholder:

`Paste your code here...`

### Missing Paste

Title:

`Paste not found`

Description:

"This paste may have expired, been removed, or the URL may be incorrect."

CTA:

`Create a new paste`

### Expired Paste

Title:

`This paste has expired`

Description:

"The paste is no longer available."

### Invalid Submission

Show inline validation messages.

### Server Error

Title:

`Something went wrong`

Description:

"We couldn't process your request. Please try again."

## 14. Design Constraints

Avoid:

- Excessive gradients.
- Glassmorphism everywhere.
- Large decorative illustrations.
- Unnecessary dashboard cards.
- Login and registration UI.
- Social media-style elements.
- Overly rounded components.
- Heavy animation.
- Excessive frontend dependencies.

## 15. Final Design Direction

Snippit should look and feel like a professional developer utility rather than a generic consumer web application.

The overall experience should be:

- Dark by default.
- Emerald-accented.
- Typography-driven.
- Code-centric.
- Fast and responsive.
- Minimal but polished.

**Final design principle: Less interface. More code.**

---

**End of Design Specification**
