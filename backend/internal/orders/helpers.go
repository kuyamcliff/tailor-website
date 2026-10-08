package orders

import (
	"context"

	"github.com/google/uuid"

	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
)

func authFrom(ctx context.Context) *auth.Principal { return auth.FromContext(ctx) }

func actorID(ctx context.Context) *uuid.UUID { return auth.ActorID(ctx) }
