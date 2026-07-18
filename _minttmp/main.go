package main

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
)

func main() {
	secret := []byte("YzCIsiVu7uAM13dR6/m0t5dq+v8/Oi+mze8mUWmhvj0=")
	kr := auth.NewKeyRotation("key-dev", secret)
	uid := uuid.MustParse("067efafa-9a58-4648-8f88-f4f099a29ce7")
	tid := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	tok, err := kr.MintAccessTokenWithContext(context.Background(), pgtype.UUID{Bytes: uid, Valid: true}, pgtype.UUID{Bytes: tid, Valid: true})
	if err != nil {
		fmt.Println("ERR", err); return
	}
	fmt.Println(tok)
}
