# 1. Levantar infraestructura + servicios
docker compose up -d --build

# 2. Ver logs de un servicio específico
docker compose logs -f auth

# 3. Reconstruir solo un servicio (caché optimizado)
docker compose up -d --build feed

# 4. Detener y limpiar volúmenes (solo dev)
docker compose down -v