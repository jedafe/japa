package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/tickets"
	"github.com/jedafe/japa/internal/worker"
)

// supportTicketBridge adapts *tickets.Service to worker.SupportTicketBridge
// for the §38 inbound email bridge (support:poll_inbox). The worker package
// cannot import internal/tickets — import cycle through internal/auth — so
// the adapter lives here and is registered at bootstrap, same pattern as
// the §06 submission linker.
type supportTicketBridge struct {
	svc *tickets.Service
}

func (b supportTicketBridge) TicketTypeID(ctx context.Context, slug string) (pgtype.UUID, error) {
	t, err := b.svc.GetTicketTypeBySlug(ctx, slug)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return t.ID, nil
}

func (b supportTicketBridge) FileTicket(ctx context.Context, in worker.SupportTicketInput) error {
	_, err := b.svc.CreateTicket(ctx, tickets.CreateTicketInput{
		TypeID:          in.TypeID,
		Subject:         in.Subject,
		Description:     in.Description,
		SubmitterUserID: in.SubmitterUserID,
		SubmitterEmail:  in.SubmitterEmail,
	})
	return err
}

func (b supportTicketBridge) FileComment(ctx context.Context, ref string, userID pgtype.UUID, fromEmail, body string) error {
	id, err := b.svc.ResolveTicketRef(ctx, ref)
	if err != nil {
		return fmt.Errorf("resolving ticket ref %q: %w", ref, err)
	}
	// Only the ticket's submitter may thread a reply onto it — the From
	// header is unauthenticated, so a resolved account alone can't prove
	// the sender may comment on this particular ticket. The email match
	// covers tickets filed before the submitter had an account.
	ticket, err := b.svc.GetTicketPrimary(ctx, id)
	if err != nil {
		return fmt.Errorf("loading ticket %q: %w", ref, err)
	}
	submitter := (userID.Valid && ticket.SubmitterUserID == userID) ||
		(ticket.SubmitterEmail != nil && strings.EqualFold(*ticket.SubmitterEmail, fromEmail))
	if !submitter {
		return fmt.Errorf("sender %q is not the submitter of %s", fromEmail, ref)
	}
	_, err = b.svc.AddComment(ctx, id, tickets.AddCommentInput{
		Body:       body,
		UserID:     userID,
		IsInternal: false,
	})
	return err
}
