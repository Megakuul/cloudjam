package game

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/internal/sortid"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/play"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/play/game"
	"connectrpc.com/connect"
	"github.com/megakuul/dynamitedb"
	"google.golang.org/protobuf/types/known/timestamppb"
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

func (s *Server) Get(ctx context.Context, req *connect.Request[game.GetRequest]) (*connect.Response[game.GetResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	gameMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Game{
		GameID: dynamitedb.Key(req.Msg.Id),
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("game does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch game: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch game"))
	}

	return &connect.Response[game.GetResponse]{Msg: &game.GetResponse{Game: &play.Game{
		Id:          gameMeta.GameID.Value(),
		Name:        gameMeta.Name.Value(),
		Description: gameMeta.Description.Value(),
		From:        timestamppb.New(gameMeta.From.Value()),
		To:          timestamppb.New(gameMeta.To.Value()),
		Scope:       gameMeta.Scope.Value(),
	}}}, nil
}

func (s *Server) List(ctx context.Context, req *connect.Request[game.ListRequest]) (*connect.Response[game.ListResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	opts := []dynamitedb.Option{dynamitedb.WithLimit(int(req.Msg.Limit))}
	if req.Msg.StartAfter != "" {
		opts = append(opts, dynamitedb.WithStartAfter(&oltp.Game{
			GameID: dynamitedb.Key(req.Msg.StartAfter),
		}))
	}
	games, err := dynamitedb.Query(ctx, s.oltp, &oltp.Game{
		GameID: dynamitedb.KeyPrefix(""),
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	}, opts...)
	if err != nil {
		l.Error(fmt.Sprintf("failed to iterate games: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to iterate games"))
	}

	gamesOutput := []*play.Game{}
	for _, game := range games {
		gamesOutput = append(gamesOutput, &play.Game{
			Id:          game.GameID.Value(),
			Name:        game.Name.Value(),
			Description: game.Description.Value(),
			From:        timestamppb.New(game.From.Value()),
			To:          timestamppb.New(game.To.Value()),
			Scope:       game.Scope.Value(),
		})
	}

	return &connect.Response[game.ListResponse]{Msg: &game.ListResponse{
		Games: gamesOutput,
	}}, nil
}

func (s *Server) Create(ctx context.Context, req *connect.Request[game.CreateRequest]) (*connect.Response[game.CreateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	if !slices.Contains(auth.Scopes(ctx), req.Msg.Init.Scope) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("can't attach a scope you don't possess"))
	}

	gameID := sortid.New().String()
	err := dynamitedb.Create(ctx, s.oltp, &oltp.Game{
		GameID:      dynamitedb.Key(gameID),
		Name:        dynamitedb.Set(req.Msg.Init.Name),
		Description: dynamitedb.Set(req.Msg.Init.Description),
		From:        dynamitedb.Set(req.Msg.Init.From.AsTime()),
		To:          dynamitedb.Set(req.Msg.Init.To.AsTime()),
		Scope:       dynamitedb.Set(req.Msg.Init.Scope),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("game does already exist"))
		}
		l.Error(fmt.Sprintf("failed to create game: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create game"))
	}
	return &connect.Response[game.CreateResponse]{Msg: &game.CreateResponse{
		Id: gameID,
	}}, nil
}

func (s *Server) Update(ctx context.Context, req *connect.Request[game.UpdateRequest]) (*connect.Response[game.UpdateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	gameMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Game{
		GameID: dynamitedb.Key(req.Msg.Mod.Id),
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
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot update running or past games"))
	}

	err = dynamitedb.Update(ctx, s.oltp, &oltp.Game{
		GameID:      dynamitedb.Key(gameMeta.GameID.Value()),
		ETag:        gameMeta.ETag,
		Name:        dynamitedb.Set(req.Msg.Mod.Name),
		Description: dynamitedb.Set(req.Msg.Mod.Description),
		From:        dynamitedb.Set(req.Msg.Mod.From.AsTime()),
		To:          dynamitedb.Set(req.Msg.Mod.To.AsTime()),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to update game: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update game"))
	}

	return &connect.Response[game.UpdateResponse]{Msg: &game.UpdateResponse{}}, nil
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[game.DeleteRequest]) (*connect.Response[game.DeleteResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	teams, err := dynamitedb.Query(ctx, s.oltp, &oltp.Team{
		GameID: dynamitedb.Key(req.Msg.Id),
		TeamID: dynamitedb.KeyPrefix(""),
	}, dynamitedb.WithLimit(1))
	if err != nil {
		l.Error(fmt.Sprintf("failed to load game teams: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load game teams"))
	}
	if len(teams) > 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("game cannot be deleted: remove all attached teams first"))
	}

	challenges, err := dynamitedb.Query(ctx, s.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(req.Msg.Id),
		ChallengeID: dynamitedb.KeyPrefix(""),
	}, dynamitedb.WithLimit(1))
	if err != nil {
		l.Error(fmt.Sprintf("failed to load game challenges: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load game challenges"))
	}
	if len(challenges) > 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("game cannot be deleted: remove all attached challenges first"))
	}

	err = dynamitedb.Delete(ctx, s.oltp, &oltp.Game{
		GameID: dynamitedb.Key(req.Msg.Id),
		To:     dynamitedb.Before(time.Now()),
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrFilterMismatch) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("game cannot be deleted"))
		}
		l.Error(fmt.Sprintf("failed to delete game: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete game"))
	}
	return &connect.Response[game.DeleteResponse]{Msg: &game.DeleteResponse{}}, nil
}
