package team

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
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/play/team"
	"connectrpc.com/connect"
	"github.com/megakuul/dynamitedb"
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

func (s *Server) Get(ctx context.Context, req *connect.Request[team.GetRequest]) (*connect.Response[team.GetResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	teamMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Team{
		GameID: dynamitedb.Key(req.Msg.GameId),
		TeamID: dynamitedb.Key(req.Msg.Id),
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("team does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch team: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch team"))
	}

	// check if the user either has access to the team scope OR has a self scope and is inside the team.
	if !slices.Contains(auth.Scopes(ctx), teamMeta.Scope.Value()) {
		userId := auth.Claims(ctx).Subject
		if _, ok := teamMeta.Players.Value()[userId]; !ok || !slices.Contains(auth.Scopes(ctx), oltp.ScopeSelf) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied"))
		}
	}

	return &connect.Response[team.GetResponse]{Msg: &team.GetResponse{Team: &play.Team{
		GameId:  teamMeta.GameID.Value(),
		Id:      teamMeta.TeamID.Value(),
		Name:    teamMeta.Name.Value(),
		Players: teamMeta.Players.Value(),
		Score:   teamMeta.Score.Value(),
		Scope:   teamMeta.Scope.Value(),
	}}}, nil
}

func (s *Server) List(ctx context.Context, req *connect.Request[team.ListRequest]) (*connect.Response[team.ListResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	opts := []dynamitedb.Option{dynamitedb.WithLimit(int(req.Msg.Limit))}
	if req.Msg.StartAfter != "" {
		opts = append(opts, dynamitedb.WithStartAfter(&oltp.Team{
			TeamID: dynamitedb.Key(req.Msg.StartAfter),
		}))
	}
	teams, err := dynamitedb.Query(ctx, s.oltp, &oltp.Team{
		GameID: dynamitedb.Key(req.Msg.GameId),
		TeamID: dynamitedb.KeyPrefix(""),
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	}, opts...)
	if err != nil {
		l.Error(fmt.Sprintf("failed to iterate teams: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to iterate teams"))
	}

	teamsOutput := []*play.Team{}
	for _, team := range teams {
		teamsOutput = append(teamsOutput, &play.Team{
			GameId:  team.GameID.Value(),
			Id:      team.TeamID.Value(),
			Name:    team.Name.Value(),
			Players: team.Players.Value(),
			Score:   team.Score.Value(),
			Scope:   team.Scope.Value(),
		})
	}

	return &connect.Response[team.ListResponse]{Msg: &team.ListResponse{
		Teams: teamsOutput,
	}}, nil
}

func (s *Server) Create(ctx context.Context, req *connect.Request[team.CreateRequest]) (*connect.Response[team.CreateResponse], error) {
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
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot create teams on a game in the past"))
	}

	teamID := sortid.New().String()
	err = dynamitedb.Create(ctx, s.oltp, &oltp.Team{
		GameID:  dynamitedb.Key(req.Msg.Init.GameId),
		TeamID:  dynamitedb.Key(teamID),
		Name:    dynamitedb.Set(req.Msg.Init.Name),
		Players: dynamitedb.Set(req.Msg.Init.Players),
		Score:   dynamitedb.Set(0.0),
		Scope:   dynamitedb.Set(req.Msg.Init.Scope),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("team does already exist"))
		}
		l.Error(fmt.Sprintf("failed to create team: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create team"))
	}
	return &connect.Response[team.CreateResponse]{Msg: &team.CreateResponse{
		Id: teamID,
	}}, nil
}

func (s *Server) Update(ctx context.Context, req *connect.Request[team.UpdateRequest]) (*connect.Response[team.UpdateResponse], error) {
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
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot update teams of a game in the past"))
	}

	teamMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Team{
		GameID: dynamitedb.Key(req.Msg.Mod.GameId),
		TeamID: dynamitedb.Key(req.Msg.Mod.Id),
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("team does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch team: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch team"))
	}

	err = dynamitedb.Update(ctx, s.oltp, &oltp.Team{
		GameID:  dynamitedb.Key(teamMeta.GameID.Value()),
		TeamID:  dynamitedb.Key(teamMeta.TeamID.Value()),
		ETag:    teamMeta.ETag,
		Name:    dynamitedb.Set(req.Msg.Mod.Name),
		Players: dynamitedb.Set(req.Msg.Mod.Players),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to update team: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update team"))
	}

	return &connect.Response[team.UpdateResponse]{Msg: &team.UpdateResponse{}}, nil
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[team.DeleteRequest]) (*connect.Response[team.DeleteResponse], error) {
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
	if time.Now().After(gameMeta.To.Value()) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot delete teams on a game in the past"))
	}

	err = dynamitedb.Delete(ctx, s.oltp, &oltp.Team{
		GameID: dynamitedb.Key(req.Msg.GameId),
		TeamID: dynamitedb.Key(req.Msg.Id),
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrFilterMismatch) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("team cannot be deleted"))
		}
		l.Error(fmt.Sprintf("failed to delete team: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete team"))
	}
	return &connect.Response[team.DeleteResponse]{Msg: &team.DeleteResponse{}}, nil
}
