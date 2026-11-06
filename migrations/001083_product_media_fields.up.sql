ALTER TABLE products ADD COLUMN description text;

-- Product gallery — ordered media set rendered on /product/{slug}.
CREATE TABLE product_gallery (
    product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    media_id   uuid NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    position   int  NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (product_id, media_id)
);
