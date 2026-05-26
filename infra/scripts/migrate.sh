#!/bin/bash
set -euo pipefail

# Configuración
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-feed_db}"
DB_USER="${DB_USER:-feed_dev}"
DB_PASS="${DB_PASS:-dev_password}"
MIGRATIONS_DIR="${MIGRATIONS_DIR:-./infra/migrations}"

# Función para ejecutar migración con goose
run_migration() {
    local direction="${1:-up}"
    
    goose -dir "$MIGRATIONS_DIR" postgres \
        "postgres://${DB_USER}:${DB_PASS}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable" \
        "$direction"
}

case "${1:-}" in
    up)
        echo "🔄 Ejecutando migraciones hacia arriba..."
        run_migration up
        ;;
    down)
        echo "⬇️  Revertiendo última migración..."
        run_migration down
        ;;
    status)
        echo "📊 Estado de migraciones:"
        run_migration status
        ;;
    create)
        if [ -z "${2:-}" ]; then
            echo "❌ Uso: $0 create <nombre_migracion>"
            exit 1
        fi
        echo "✨ Creando migración: $2"
        goose -dir "$MIGRATIONS_DIR" create "$2" sql
        ;;
    *)
        echo "Uso: $0 {up|down|status|create <name>}"
        exit 1
        ;;
esac
