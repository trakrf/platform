'use strict';

// trakrf-readonly-not-in-write-required (Spectral custom function)
//
// For each `Create.+Request` / `Update.+Request` / `Rename.+Request` schema
// under components.schemas, flag any field in the schema's `required[]` that
// is `readOnly: true` on the paired `*View` schema. Round-trip
// GET -> mutate -> POST/PATCH should never require an integrator to filter
// readOnly fields out of a write-side payload.
//
// Pairing heuristic:
//   1. Strip leading `Create|Update|Rename` and trailing `Request`
//      (e.g. `CreateAssetWithTagsRequest` -> `AssetWithTags`).
//   2. First try `${core}View`. If that schema does not exist, retry after
//      stripping common infixes (`WithTags`) so request variants pair to
//      the base view (e.g. `AssetWithTags` -> `Asset` -> `AssetView`).
//   3. If no paired view is found, the schema is skipped (no false positives
//      on requests whose resource has no View counterpart, e.g. TagRequest).
//
// Origin: BB29 F11 — readOnly/required asymmetry. Tracked under TRA-690
// (Tier 1 gates).

const REQUEST_NAME_RE = /^(Create|Update|Rename)(.+)Request$/;

const candidateViews = (core) => {
  const names = [`${core}View`];
  const stripped = core.replace(/WithTags$/, '');
  if (stripped !== core) names.push(`${stripped}View`);
  return names;
};

export default (schemas) => {
  if (!schemas || typeof schemas !== 'object') return [];

  const results = [];

  for (const [name, schema] of Object.entries(schemas)) {
    const m = name.match(REQUEST_NAME_RE);
    if (!m) continue;

    const required = Array.isArray(schema?.required) ? schema.required : [];
    if (required.length === 0) continue;

    let pairedName = null;
    let pairedSchema = null;
    for (const cand of candidateViews(m[2])) {
      if (schemas[cand]) {
        pairedName = cand;
        pairedSchema = schemas[cand];
        break;
      }
    }
    if (!pairedSchema) continue;

    const props = pairedSchema.properties ?? {};
    for (let i = 0; i < required.length; i++) {
      const fieldName = required[i];
      const prop = props[fieldName];
      if (prop && prop.readOnly === true) {
        results.push({
          message: `${name}.required includes '${fieldName}', which is readOnly:true on ${pairedName} — remove from write-side required (BB29 F11)`,
          path: ['components', 'schemas', name, 'required', i],
        });
      }
    }
  }

  return results;
};
