#!/usr/bin/env bash
set -Eeuo pipefail

for name in PGHOST PGDATABASE PGUSER PGPASSWORD; do
  if [[ -z "${!name:-}" ]]; then
    echo "Variavel obrigatoria ausente no migrador: ${name}" >&2
    exit 1
  fi
done

migrations=(/opt/lice/migrations/*.up.sql)
if [[ ! -e "${migrations[0]}" ]]; then
  echo "Nenhuma migration .up.sql encontrada." >&2
  exit 1
fi

declare -A seen_versions=()
versions=()
filenames=()
checksums=()
for migration in "${migrations[@]}"; do
  filename="$(basename "$migration")"
  if [[ ! "$filename" =~ ^([0-9]{6})_[a-z0-9][a-z0-9_-]*\.up\.sql$ ]]; then
    echo "Nome de migration invalido: ${filename}" >&2
    exit 1
  fi
  raw_version="${BASH_REMATCH[1]}"
  version=$((10#$raw_version))
  if (( version < 1 )); then
    echo "A versao da migration deve ser positiva: ${filename}" >&2
    exit 1
  fi
  if [[ -n "${seen_versions[$version]:-}" ]]; then
    echo "Versao de migration duplicada: ${filename} e ${seen_versions[$version]}" >&2
    exit 1
  fi
  seen_versions["$version"]="$filename"
  versions+=("$version")
  filenames+=("$filename")
  checksums+=("$(sha256sum "$migration" | cut -d ' ' -f 1)")
done

plan="$(mktemp /tmp/lice-migrations.XXXXXX.sql)"
cleanup() {
  rm -f "$plan"
}
trap cleanup EXIT

{
  cat <<'SQL'
\set ON_ERROR_STOP on
SELECT pg_advisory_lock(hashtextextended('lice.schema_migrations', 0));
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    version bigint PRIMARY KEY,
    filename text NOT NULL UNIQUE,
    checksum text,
    applied_at timestamptz NOT NULL DEFAULT statement_timestamp()
);
ALTER TABLE public.schema_migrations ADD COLUMN IF NOT EXISTS checksum text;
REVOKE ALL ON public.schema_migrations FROM PUBLIC, lice_runtime;
SQL

  for index in "${!versions[@]}"; do
    version="${versions[$index]}"
    filename="${filenames[$index]}"
    checksum="${checksums[$index]}"
    migration="/opt/lice/migrations/$filename"
    printf "UPDATE public.schema_migrations SET checksum = '%s' WHERE version = %s AND filename = '%s' AND checksum IS NULL;\n" "$checksum" "$version" "$filename"
    printf "DO \$verify\$ BEGIN IF EXISTS (SELECT 1 FROM public.schema_migrations WHERE version = %s AND (filename <> '%s' OR checksum <> '%s')) THEN RAISE EXCEPTION 'migration %% divergiu do registro aplicado', %s; END IF; END \$verify\$;\n" "$version" "$filename" "$checksum" "$version"
    printf "SELECT EXISTS (SELECT 1 FROM public.schema_migrations WHERE version = %s) AS migration_applied \\gset\n" "$version"
    printf '\\if :migration_applied\n'
    printf '\\else\n'
    printf 'BEGIN;\n'
    printf '\\i %s\n' "$migration"
    printf "INSERT INTO public.schema_migrations (version, filename, checksum) VALUES (%s, '%s', '%s');\n" "$version" "$filename" "$checksum"
    printf 'COMMIT;\n'
    printf '\\endif\n'
  done

  cat <<'SQL'
ALTER TABLE public.schema_migrations ALTER COLUMN checksum SET NOT NULL;
SELECT pg_advisory_unlock(hashtextextended('lice.schema_migrations', 0));
SQL
} >"$plan"

psql --quiet --file="$plan"
trap - EXIT
rm -f "$plan"

echo "Migrations aplicadas com sucesso."
