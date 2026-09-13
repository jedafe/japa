#!/usr/bin/env python3
"""Remove the leading tenant argument from video GetSourceByID calls and signatures."""
from pathlib import Path
import re

files = [
    "internal/video/store.go",
    "internal/video/service.go",
    "internal/video/handlers.go",
    "internal/video/service_test.go",
]

for f in files:
    p = Path(f)
    text = p.read_text()

    # Signature in store.go
    text = re.sub(
        r"func \(s \*Store\) GetSourceByID\(ctx context\.Context, tenantID, id pgtype\.UUID\)",
        "func (s *Store) GetSourceByID(ctx context.Context, id pgtype.UUID)",
        text,
    )

    # Signature in service.go
    text = re.sub(
        r"func \(s \*Service\) GetSourceByID\(ctx context\.Context, tenantID, id pgtype\.UUID\)",
        "func (s *Service) GetSourceByID(ctx context.Context, id pgtype.UUID)",
        text,
    )

    # Calls
    text = re.sub(
        r"(s\.store\.GetSourceByID|s\.Service\.GetSourceByID|h\.Service\.GetSourceByID)\(ctx, tenantID, (\w+)\)",
        r"\1(ctx, \2)",
        text,
    )

    # test calls
    text = re.sub(
        r"videoStore\.GetSourceByID\(context\.Background\(\), tenantID, (\w+)\)",
        r"videoStore.GetSourceByID(context.Background(), \1)",
        text,
    )

    p.write_text(text)
    print("patched", f)
