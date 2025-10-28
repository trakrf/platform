# Database Seeds

This directory contains seed data SQL files for different environments.

## Files

- `development.sql` - Sample data for local development

## Usage

### Seed the database

```bash
just backend seed
```

### Reset database (drop, create, migrate, seed)

```bash
just backend db-reset
just backend migrate
just backend seed
```

## Seed Data Contents

### development.sql

- **Organizations**: ACME Corporation, Test Organization
- **Users**: admin@acme.com, user@acme.com, test@test.com (all with password 'password')
- **Assets**: 3 sample assets (laptops, phones, people)
- **Locations**: Warehouse A, Office Building
- **Scan Devices**: 1 fixed scanner

## Notes

- All seed files use `ON CONFLICT DO NOTHING` to be idempotent
- Password hashes are example hashes - replace with real hashes in production
- Safe to run multiple times
