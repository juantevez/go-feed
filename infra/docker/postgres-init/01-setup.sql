-- Se ejecuta automáticamente en el primer docker-compose up
-- Crear database y usuario si no existen

DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_database WHERE datname = 'feed_db') THEN
        CREATE DATABASE feed_db;
    END IF;
    
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'feed_dev') THEN
        CREATE ROLE feed_dev WITH LOGIN PASSWORD 'dev_password';
    END IF;
    
    GRANT ALL PRIVILEGES ON DATABASE feed_db TO feed_dev;
END $$;
