package challenge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/play"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/play/challenge"
	"connectrpc.com/connect"
	"github.com/megakuul/dynamitedb"
	"google.golang.org/protobuf/types/known/durationpb"
)

type Server struct {
	logger *slog.Logger
	oltp   *dynamitedb.Bucket
}

func New(logger *slog.Logger, oltp *dynamitedb.Bucket) *Server {
	return &Server{
		logger: logger,
		oltp:   oltp,
	}
}

func (s *Server) Get(ctx context.Context, req *connect.Request[challenge.GetRequest]) (*connect.Response[challenge.GetResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	challengeMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(req.Msg.GameId),
		ChallengeID: dynamitedb.Key(req.Msg.Id),
		Scope:       dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("challenge does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch challenge: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch challenge"))
	}

	coveredClues := challengeMeta.Clues.Value()
	for clue := range coveredClues {
		if !challengeMeta.UncoveredClues.Value()[clue] {
			coveredClues[clue] = "<hidden>"
		}
	}

	return &connect.Response[challenge.GetResponse]{Msg: &challenge.GetResponse{Challenge: &play.Challenge{
		GameId:               challengeMeta.GameID.Value(),
		Id:                   challengeMeta.ChallengeID.Value(),
		TeamId:               challengeMeta.TeamID.Value(),
		DefinitionId:         challengeMeta.DefinitionID.Value(),
		DefinitionProviderId: challengeMeta.DefinitionProviderID.Value(),
		Title:                challengeMeta.Title.Value(),
		Description:          challengeMeta.Description.Value(),
		Assets:               challengeMeta.Assets.Value(),
		Clues:                coveredClues,
		Errors:               challengeMeta.Errors.Value(),
		ScoreEvents:          challengeMeta.ScoreEvents.Value(),
		Duration:             durationpb.New(challengeMeta.Duration.Value()),
		Scope:                challengeMeta.Scope.Value(),
	}}}, nil
}

func (s *Server) List(ctx context.Context, req *connect.Request[challenge.ListRequest]) (*connect.Response[challenge.ListResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	opts := []dynamitedb.Option{dynamitedb.WithLimit(int(req.Msg.Limit))}
	if req.Msg.StartAfter != "" {
		opts = append(opts, dynamitedb.WithStartAfter(&oltp.Challenge{
			ChallengeID: dynamitedb.Key(req.Msg.StartAfter),
		}))
	}
	challenges, err := dynamitedb.Query(ctx, s.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(req.Msg.GameId),
		ChallengeID: dynamitedb.KeyPrefix(""),
		Scope:       dynamitedb.In(auth.Scopes(ctx)...),
	}, opts...)
	if err != nil {
		l.Error(fmt.Sprintf("failed to iterate challenges: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to iterate challenges"))
	}

	challengesOutput := []*play.Challenge{}
	for _, challenge := range challenges {
		coveredClues := challenge.Clues.Value()
		for clue := range coveredClues {
			if !challenge.UncoveredClues.Value()[clue] {
				coveredClues[clue] = "<hidden>"
			}
		}
		challengesOutput = append(challengesOutput, &play.Challenge{
			GameId:               challenge.GameID.Value(),
			Id:                   challenge.ChallengeID.Value(),
			TeamId:               challenge.TeamID.Value(),
			DefinitionId:         challenge.DefinitionID.Value(),
			DefinitionProviderId: challenge.DefinitionProviderID.Value(),
			Title:                challenge.Title.Value(),
			Description:          challenge.Description.Value(),
			Assets:               challenge.Assets.Value(),
			Clues:                coveredClues,
			Errors:               challenge.Errors.Value(),
			ScoreEvents:          challenge.ScoreEvents.Value(),
			Duration:             durationpb.New(challenge.Duration.Value()),
			Scope:                challenge.Scope.Value(),
		})
	}

	return &connect.Response[challenge.ListResponse]{Msg: &challenge.ListResponse{
		Challenges: challengesOutput,
	}}, nil
}

func (s *Server) Create(ctx context.Context, req *connect.Request[challenge.CreateRequest]) (*connect.Response[challenge.CreateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	if !slices.Contains(auth.Scopes(ctx), req.Msg.Init.Scope) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("can't attach a scope you don't possess"))
	}

	gameMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Game{
		GameID: dynamitedb.Key(req.Msg.Init.GameId),
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("game does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch game: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch game"))
	}
	if time.Now().After(gameMeta.To.Value()) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot create challenges on a game in the past"))
	}

	err = dynamitedb.Create(ctx, s.oltp, &oltp.Challenge{
		GameID:               dynamitedb.Key(req.Msg.Init.GameId),
		ChallengeID:          dynamitedb.Key(req.Msg.Init.Id),
		TeamID:               dynamitedb.Set(req.Msg.Init.TeamId),
		DefinitionProviderID: dynamitedb.Set(req.Msg.Init.DefinitionProviderId),
		DefinitionID:         dynamitedb.Set(req.Msg.Init.DefinitionId),

		Scope: dynamitedb.Set(req.Msg.Init.Scope),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("challenge does already exist"))
		}
		l.Error(fmt.Sprintf("failed to create challenge: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create challenge"))
	}
	return &connect.Response[challenge.CreateResponse]{Msg: &challenge.CreateResponse{}}, nil
}

func (s *Server) Update(ctx context.Context, req *connect.Request[challenge.UpdateRequest]) (*connect.Response[challenge.UpdateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	gameMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Game{
		GameID: dynamitedb.Key(req.Msg.Mod.GameId),
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("game does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch game: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch game"))
	}
	if time.Now().After(gameMeta.To.Value()) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot update challenges of a game in the past"))
	}

	challengeMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(req.Msg.Mod.GameId),
		ChallengeID: dynamitedb.Key(req.Msg.Mod.Id),
		Scope:       dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("challenge does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch challenge: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch challenge"))
	}

	err = dynamitedb.Update(ctx, s.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(challengeMeta.GameID.Value()),
		ChallengeID: dynamitedb.Key(challengeMeta.ChallengeID.Value()),
		ETag:        challengeMeta.ETag,
		TeamID:      dynamitedb.Set(req.Msg.Mod.TeamId),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to update challenge: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update challenge"))
	}

	return &connect.Response[challenge.UpdateResponse]{Msg: &challenge.UpdateResponse{}}, nil
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[challenge.DeleteRequest]) (*connect.Response[challenge.DeleteResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	gameMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Game{
		GameID: dynamitedb.Key(req.Msg.GameId),
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("game does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch game: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch game"))
	}
	if time.Now().After(gameMeta.From.Value()) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot delete challenges on a past or running game"))
	}

	err = dynamitedb.Delete(ctx, s.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(req.Msg.GameId),
		ChallengeID: dynamitedb.Key(req.Msg.Id),
		Scope:       dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrFilterMismatch) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("challenge cannot be deleted"))
		}
		l.Error(fmt.Sprintf("failed to delete challenge: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete challenge"))
	}
	return &connect.Response[challenge.DeleteResponse]{Msg: &challenge.DeleteResponse{}}, nil
}
