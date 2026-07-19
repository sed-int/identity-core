// Package grpc is the board's delivery layer.
package grpc

import (
	"context"
	"errors"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	boardv1 "identity-service/api/gen/board/v1"
	"identity-service/pkg/jwks"
	"identity-service/services/board/internal/domain"
	"identity-service/services/board/internal/usecase"
)

type Handler struct {
	boardv1.UnimplementedBoardServiceServer
	board *usecase.Board
}

func NewHandler(board *usecase.Board) *Handler {
	return &Handler{board: board}
}

// NewServer builds the board gRPC server with auth + logging interceptors.
func NewServer(log zerolog.Logger, board *usecase.Board, verifier *jwks.Verifier) *grpc.Server {
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(
		LoggingInterceptor(log),
		AuthInterceptor(verifier),
	))
	boardv1.RegisterBoardServiceServer(s, NewHandler(board))
	return s
}

func (h *Handler) CreatePost(ctx context.Context, req *boardv1.CreatePostRequest) (*boardv1.CreatePostResponse, error) {
	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing claims") // interceptor guarantees this
	}
	post, err := h.board.CreatePost(ctx, claims.Subject, req.GetTitle(), req.GetContent())
	if err != nil {
		return nil, mapErr(err)
	}
	return &boardv1.CreatePostResponse{Post: toProto(post)}, nil
}

func (h *Handler) ListPosts(ctx context.Context, req *boardv1.ListPostsRequest) (*boardv1.ListPostsResponse, error) {
	posts, next, err := h.board.ListPosts(ctx, req.GetPageAfter(), int(req.GetPageSize()))
	if err != nil {
		return nil, mapErr(err)
	}
	resp := &boardv1.ListPostsResponse{NextPageAfter: next}
	for _, p := range posts {
		resp.Posts = append(resp.Posts, toProto(p))
	}
	return resp, nil
}

func toProto(p *domain.Post) *boardv1.Post {
	return &boardv1.Post{
		Id:             p.ID,
		AuthorId:       p.AuthorID,
		Title:          p.Title,
		Content:        p.Content,
		CreatedAt:      timestamppb.New(p.CreatedAt),
		AuthorNickname: p.AuthorNickname,
	}
}

func mapErr(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidPost):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrPostNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrNotAuthorized):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
