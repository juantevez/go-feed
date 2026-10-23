-- Recalcula mutual en las relaciones existentes. Hasta este fix, un follow que
-- completaba una relación mutua marcaba solo la fila nueva: la del otro lado
-- quedaba en false. Desde acá UpdateMutual actualiza las dos filas.
UPDATE follows f
SET mutual = EXISTS (
    SELECT 1 FROM follows r
    WHERE r.follower_id = f.following_id AND r.following_id = f.follower_id
);
