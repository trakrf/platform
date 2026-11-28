/**
 * An asset is a person when its metadata carries `person: true` — the flag
 * mustering, the location map's people/assets split and presence all read
 * (`metadata->>'person' = 'true'` server-side).
 *
 * The asset PATCH replaces metadata wholesale, so the form must send back the
 * keys it was given; it only owns the person flag.
 */
export function isPersonAsset(metadata: Record<string, unknown> | null | undefined): boolean {
  return metadata?.person === true || metadata?.person === 'true';
}

export function withPersonFlag(
  metadata: Record<string, unknown> | null | undefined,
  person: boolean,
): Record<string, unknown> {
  const { person: _drop, ...rest } = metadata ?? {};
  return person ? { ...rest, person: true } : rest;
}
