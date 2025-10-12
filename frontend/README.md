# Frontend (React)

React web application for TrakRF platform.

## Structure

```
frontend/
├── src/
│   ├── components/  # Reusable UI components
│   ├── pages/       # Page-level components
│   ├── services/    # API clients
│   ├── store/       # State management
│   ├── types/       # TypeScript type definitions
│   └── utils/       # Helper functions
├── public/          # Static assets
└── tests/           # Test files
```

## Development

### Prerequisites
- Node.js 18+
- pnpm (REQUIRED - do not use npm or npx)

### Setup
```bash
cd frontend

# Install dependencies (use pnpm ONLY)
pnpm install

# Start dev server
pnpm dev

# Run tests
pnpm test

# Build for production
pnpm build
```

## Package Manager

**IMPORTANT**: This project uses `pnpm` EXCLUSIVELY.

✅ Correct:
```bash
pnpm install
pnpm run lint
pnpm test
```

❌ Incorrect (never use):
```bash
npm install    # NO
npx vite       # Use: pnpm dlx vite
```

## Validation

### From frontend/ directory:
```bash
# Lint
pnpm run lint --fix

# Typecheck
pnpm run typecheck

# Test
pnpm test

# Build
pnpm run build
```

### From project root (via Just):
```bash
just frontend-lint
just frontend-typecheck
just frontend-test
just frontend-build
just frontend  # All frontend checks
```

## Tech Stack

- **Framework**: React + TypeScript
- **Build Tool**: Vite
- **Test Runner**: Vitest
- **Styling**: (TBD - likely Tailwind CSS)
- **State**: (TBD - likely Zustand or React Context)

## Architecture

- **Component-driven** - Modular, reusable UI components
- **Type-safe** - Full TypeScript coverage
- **Test coverage** - Unit and integration tests for business logic
