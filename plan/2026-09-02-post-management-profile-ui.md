# Plan: Post management, persisted profile, and frontend refresh

> Status: 🚧 in progress
> Branch: `feat/post-management-ui`

## Goal

Turn the Phase 5 demo into a small but coherent authenticated community app:

- post authors can update and delete only their own posts;
- signup collects the PRD-defined display profile (`nickname` and optional profile image URL) and persists it in Identity;
- authenticated users can retrieve and view their profile after signup or session restoration; and
- auth, Board, and profile screens have a responsive, accessible visual system with clear loading, empty, error, edit, and destructive-action states.

## Contract and backend

1. Extend `board.v1.BoardService` with authenticated `PATCH /board/v1/posts/{id}` and `DELETE /board/v1/posts/{id}` operations.
2. Keep authorship server-controlled: derive the actor from the verified access-token `sub`; never accept an author ID from the client.
3. Add use-case ownership checks and repository update/delete operations. Return `PERMISSION_DENIED` for another user's post and `NOT_FOUND` for a missing post.
4. Extend `identity.v1.IdentityService` with authenticated `GET /auth/v1/me`.
5. Verify `/me` access tokens locally with the Identity signing key, then load account/profile data from Identity's own repository. Expose user ID, status, created time, nickname, image URL, and reputation score; do not expose the phone credential.
6. Keep signup persistence on the existing transactional `users` + `user_credentials` + `user_profiles` + outbox path. Add the existing optional `profile_image_url` field to the browser signup form.
7. Regenerate Go, gRPC-gateway, and OpenAPI outputs from the proto contracts.

## Frontend experience

1. Replace the minimal demo styling with a warm, high-contrast community workspace that remains compact on mobile.
2. Improve OTP and signup screens with step context, field guidance, visible progress/error states, and a profile-image preview/fallback.
3. Add an authenticated application shell with Board/Profile navigation and a signed-in identity summary.
4. Show edit/delete controls only on posts owned by the current user. Editing happens inline; deletion requires an explicit in-card confirmation.
5. Add loading, empty, mutation-error, pagination, and session-restoration states without introducing a new state-management dependency.

## Verification

- Unit-test Board ownership, update, and delete behavior.
- Test Identity access-token verification and current-profile retrieval.
- Extend frontend API tests for `/me`, update, and delete request behavior.
- Run `make proto`, `gofmt`, `go test ./...`, `npm test`, and `npm run build`.
- Run the Compose stack, migrations, and a browser walkthrough covering signup/profile/create/edit/delete.

## Out of scope

- Profile editing or image upload/storage (signup accepts an optional hosted image URL).
- Comments, reactions, moderation, or admin post deletion.
- Exposing private phone credentials through the profile API.
