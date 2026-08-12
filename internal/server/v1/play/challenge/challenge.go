package challenge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/auth"
	challengerunner "codeberg.org/megakuul/cloudjam/internal/challenge"
	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/internal/provider/cache"
	"codeberg.org/megakuul/cloudjam/internal/scheduler"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/play"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/play/challenge"
	"connectrpc.com/connect"
	"github.com/megakuul/dynamitedb"
	"github.com/megakuul/lake"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	logger      *slog.Logger
	scheduler   *scheduler.Scheduler
	providers   *cache.Cache
	pluginCache *challengerunner.Cache
	oltp        *dynamitedb.Bucket
	olap        *lake.Bucket
}

func New(logger *slog.Logger, scheduler *scheduler.Scheduler, providers *cache.Cache, pluginCache *challengerunner.Cache, oltp *dynamitedb.Bucket, olap *lake.Bucket) *Server {
	return &Server{
		logger:      logger,
		scheduler:   scheduler,
		providers:   providers,
		pluginCache: pluginCache,
		oltp:        oltp,
		olap:        olap,
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

func (s *Server) Start(ctx context.Context, req *connect.Request[challenge.StartRequest]) (*connect.Response[challenge.StartResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	gameMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Game{
		GameID: dynamitedb.Key(req.Msg.GameId),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to fetch game: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch game"))
	}
	if time.Now().Before(gameMeta.From.Value()) || time.Now().After(gameMeta.To.Value()) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("game is not running"))
	}

	challengeMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(gameMeta.GameID.Value()),
		ChallengeID: dynamitedb.Key(req.Msg.Id),
		Scope:       dynamitedb.Eq(gameMeta.Scope.Value()),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to fetch challenge: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch challenge"))
	}

	teamMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Team{
		GameID: dynamitedb.Key(gameMeta.GameID.Value()),
		TeamID: dynamitedb.Key(challengeMeta.TeamID.Value()),
		Scope:  dynamitedb.Eq(gameMeta.Scope.Value()),
	})

	// check if the user either has access to the game scope OR has a self scope and is inside the challenge team.
	if !slices.Contains(auth.Scopes(ctx), gameMeta.Scope.Value()) {
		userId := auth.Claims(ctx).Subject
		if _, ok := teamMeta.Players.Value()[userId]; !ok || !slices.Contains(auth.Scopes(ctx), oltp.ScopeSelf) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied"))
		}
	}

	definitionMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Definition{
		ProviderID:   dynamitedb.Key(challengeMeta.DefinitionProviderID.Value()),
		DefinitionID: dynamitedb.Key(challengeMeta.DefinitionID.Value()),
		Scope:        dynamitedb.Eq(challengeMeta.Scope.Value()),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to fetch challenge definition: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch challenge definition"))
	}

	accounts, err := dynamitedb.Query(ctx, s.oltp, &oltp.Account{
		ProviderID: dynamitedb.Key(definitionMeta.ProviderID.Value()),
		AccountID:  dynamitedb.KeyPrefix(""),
		State:      dynamitedb.Eq(cloud.AccountState_Ready),
		BoundUntil: dynamitedb.Before(time.Now()),
		Scope:      dynamitedb.Eq(definitionMeta.Scope.Value()),
	}, dynamitedb.WithLimit(1))
	if err != nil {
		l.Error(fmt.Sprintf("failed to enumerate accounts: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to enumerate accounts"))
	}
	if len(accounts) < 1 {
		return nil, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("no capacity: not enough accounts on challenge provider"))
	}
	account := accounts[0]
	err = dynamitedb.Update(ctx, s.oltp, &oltp.Account{
		ProviderID: dynamitedb.Key(account.ProviderID.Value()),
		AccountID:  dynamitedb.Key(account.AccountID.Value()),
		ETag:       account.ETag,
		State:      dynamitedb.Set(cloud.AccountState_Running),
		BoundUntil: dynamitedb.Set(gameMeta.To.Value()),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to claim account: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to claim account"))
	}

	providerMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Provider{
		ProviderID: dynamitedb.Key(challengeMeta.DefinitionProviderID.Value()),
		Scope:      dynamitedb.Eq(gameMeta.Scope.Value()),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to fetch challenge provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch challenge provider"))
	}

	provider, err := s.providers.Load(ctx, providerMeta)
	if err != nil {
		l.Error(fmt.Sprintf("failed to load challenge provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load challenge provider"))
	}

	access, err := provider.Access(ctx, account.AccountID.Value(), time.Until(gameMeta.To.Value()))
	if err != nil {
		l.Error(fmt.Sprintf("failed to create access controller: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create access controller"))
	}
	assets, err := provider.Assets(ctx, account.AccountID.Value(), time.Until(gameMeta.To.Value()))
	if err != nil {
		l.Error(fmt.Sprintf("failed to create asset controller: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create asset controller"))
	}
	resources, err := provider.Resources(ctx, account.AccountID.Value(), time.Until(gameMeta.To.Value()))
	if err != nil {
		l.Error(fmt.Sprintf("failed to create resource controller: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create resource controller"))
	}

	challengeRunner := challengerunner.New(l,
		definitionMeta, challengeMeta, teamMeta,
		s.pluginCache, s.oltp, s.olap, access, assets, resources,
	)

	s.scheduler.Schedule(func(ctx context.Context) error {
		ctx, cancel := context.WithDeadline(ctx, gameMeta.To.Value())
		defer cancel()
		if err := challengeRunner.Start(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return nil
	}, func(ctx context.Context, err error) error {
		l.Warn(fmt.Sprintf("failed to start challenge (%q): %v", challengeMeta.ChallengeID.Value(), err))
		return dynamitedb.Update(ctx, s.oltp, &oltp.Challenge{
			GameID:      dynamitedb.Key(challengeMeta.GameID.Value()),
			ChallengeID: dynamitedb.Key(challengeMeta.ChallengeID.Value()),
			Errors:      dynamitedb.Append(err.Error()),
		})
	})

	return &connect.Response[challenge.StartResponse]{Msg: &challenge.StartResponse{}}, nil
}

func (s *Server) Credentials(ctx context.Context, req *connect.Request[challenge.CredentialsRequest]) (*connect.Response[challenge.CredentialsResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	gameMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Game{
		GameID: dynamitedb.Key(req.Msg.GameId),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to fetch game: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch game"))
	}
	if time.Now().Before(gameMeta.From.Value()) || time.Now().After(gameMeta.To.Value()) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("game is not running"))
	}

	challengeMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(gameMeta.GameID.Value()),
		ChallengeID: dynamitedb.Key(req.Msg.Id),
		Scope:       dynamitedb.Eq(gameMeta.Scope.Value()),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to fetch challenge: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch challenge"))
	}

	teamMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Team{
		GameID: dynamitedb.Key(gameMeta.GameID.Value()),
		TeamID: dynamitedb.Key(challengeMeta.TeamID.Value()),
		Scope:  dynamitedb.Eq(gameMeta.Scope.Value()),
	})

	// check if the user either has access to the game scope OR has a self scope and is inside the challenge team.
	if !slices.Contains(auth.Scopes(ctx), gameMeta.Scope.Value()) {
		userId := auth.Claims(ctx).Subject
		if _, ok := teamMeta.Players.Value()[userId]; !ok || !slices.Contains(auth.Scopes(ctx), oltp.ScopeSelf) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied"))
		}
	}

	providerMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Provider{
		ProviderID: dynamitedb.Key(challengeMeta.DefinitionProviderID.Value()),
		Scope:      dynamitedb.Eq(gameMeta.Scope.Value()),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to fetch challenge provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch challenge provider"))
	}

	provider, err := s.providers.Load(ctx, providerMeta)
	if err != nil {
		l.Error(fmt.Sprintf("failed to load challenge provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load challenge provider"))
	}

	credentials, err := provider.Credentials(ctx, challengeMeta.AccountID.Value(), time.Until(gameMeta.To.Value()))
	if err != nil {
		l.Error(fmt.Sprintf("failed to generate challenge credentials: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate challenge credentials"))
	}

	return &connect.Response[challenge.CredentialsResponse]{Msg: &challenge.CredentialsResponse{
		Credentials: credentials,
	}}, nil
}

func (s *Server) UncoverClue(ctx context.Context, req *connect.Request[challenge.UncoverClueRequest]) (*connect.Response[challenge.UncoverClueResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	gameMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Game{
		GameID: dynamitedb.Key(req.Msg.GameId),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to fetch game: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch game"))
	}
	if time.Now().Before(gameMeta.From.Value()) || time.Now().After(gameMeta.To.Value()) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("game is not running"))
	}

	challengeMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(gameMeta.GameID.Value()),
		ChallengeID: dynamitedb.Key(req.Msg.Id),
		Scope:       dynamitedb.Eq(gameMeta.Scope.Value()),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to fetch challenge: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch challenge"))
	}

	teamMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Team{
		GameID: dynamitedb.Key(gameMeta.GameID.Value()),
		TeamID: dynamitedb.Key(challengeMeta.TeamID.Value()),
		Scope:  dynamitedb.Eq(gameMeta.Scope.Value()),
	})

	// check if the user either has access to the game scope OR has a self scope and is inside the challenge team.
	if !slices.Contains(auth.Scopes(ctx), gameMeta.Scope.Value()) {
		userId := auth.Claims(ctx).Subject
		if _, ok := teamMeta.Players.Value()[userId]; !ok || !slices.Contains(auth.Scopes(ctx), oltp.ScopeSelf) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied"))
		}
	}

	price, ok := challengeMeta.CluePrices.Value()[req.Msg.Clue]
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("clue '%s' does not exist", req.Msg.Clue))
	}
	err = dynamitedb.Update(ctx, s.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(challengeMeta.GameID.Value()),
		ChallengeID: dynamitedb.Key(challengeMeta.ChallengeID.Value()),
		ScoreEvents: dynamitedb.Append(&play.ScoreEvent{
			Timestamp: timestamppb.Now(),
			Text:      fmt.Sprintf("Clue '%s' uncovered", req.Msg.Clue),
			Change:    price,
		}),
		UncoveredClues: dynamitedb.Emplace(map[string]bool{
			req.Msg.Clue: true,
		}),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to update challenge: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update challenge"))
	}
	err = dynamitedb.Update(ctx, s.oltp, &oltp.Team{
		GameID: dynamitedb.Key(challengeMeta.GameID.Value()),
		TeamID: dynamitedb.Key(teamMeta.TeamID.Value()),
		Score:  dynamitedb.Increment(price),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to update score (clue <-> score corruption: team '%s' owes '%2f' points): %v", teamMeta.Name.Value(), price, err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update score"))
	}
	return &connect.Response[challenge.UncoverClueResponse]{Msg: &challenge.UncoverClueResponse{}}, nil
}
