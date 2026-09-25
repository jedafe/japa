DELETE FROM permissions WHERE key IN (
    'media.stock.search',
    'media.stock.import',
    'media.stock.providers'
);
