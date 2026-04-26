# Sales Analyzer — API Agent

## Role
You are the **API Agent**. You own everything inside `sales_analyzer_api/`. Do not touch `sales_analyzer_ui/`.

## Tech Stack
- **Language**: Go
- **Router**: `github.com/go-chi/chi/v5`
- **DB**: Supabase PostgreSQL via `github.com/jackc/pgx/v5`
- **AI**: OpenRouter (OpenAI-compatible) via `net/http` — base URL `https://openrouter.ai/api/v1`
- **Auth**: JWT (`github.com/golang-jwt/jwt/v5`) + bcrypt
- **Streaming**: SSE via `internal/sse` package

## Project Structure
```
cmd/server/main.go          — entry point, router, middleware
internal/
  auth/jwt.go               — GenerateToken, ValidateToken (Claims: user_id, email, name)
  db/db.go                  — pgx connection pool
  handlers/
    auth.go                 — Register, Login
    summary.go              — ServeHTTP + ServeSSE
    search.go               — ServeHTTP + ServeSSE
    ranking.go              — ServeHTTP + ServeSSE
    insights.go             — ServeHTTP + ServeSSE
    similar.go              — ServeHTTP + ServeSSE
    categories.go           — ServeHTTP
    helpers.go              — writeJSON, writeError
  llm/client.go             — OpenRouter client (Complete method)
  middleware/auth.go        — JWTAuth middleware
  models/
    product.go              — Product, Review, SummaryResponse, etc.
    user.go                 — User, RegisterRequest, LoginRequest, AuthResponse
  sse/sse.go                — SSE Writer, Stream helper
sql/
  001_create_users.sql
  002_create_pos_tables.sql
```

## Environment Variables
```
DATABASE_URL=postgresql://postgres.[ref]:[pass]@aws-0-ap-southeast-1.pooler.supabase.com:5432/postgres
OPENROUTER_API_KEY=sk-or-...
OPENROUTER_MODEL=anthropic/claude-sonnet-4-5
JWT_SECRET=min-32-chars
PORT=8080
```

## Coding Rules
- Always read a file before editing
- Run `go build ./...` and `go vet ./...` after every change — must pass
- Run `go mod tidy` after adding/removing dependencies
- Keep `ServeHTTP` (sync) and `ServeSSE` (streaming) on every handler
- SSE goroutines must use `ctx := r.Context()` captured before goroutine launch
- JWT Claims include: `user_id`, `email`, `name`
- All AI responses that return JSON: use `llm.JSONSystemPrompt` as system prompt
- Return proper HTTP codes: 400 bad input, 401 unauth, 404 not found, 409 conflict, 500 server error

## DB Notes (Supabase)
- `products.product_details` is JSONB — use `->>'field'` for text, `->'field'` for JSON
- `users.id` is UUID
- New POS tables: `locations`, `inventory`, `customers`, `orders`, `order_items`, `payments`

## SSE Pattern
```go
func (h *Handler) ServeSSE(w http.ResponseWriter, r *http.Request) {
    sw, ok := sse.New(w)
    if !ok { http.Error(w, "streaming unsupported", 500); return }

    ch := make(chan sse.Event, 5)
    ctx := r.Context()

    go func() {
        defer close(ch)
        ch <- sse.Event{Type: sse.EventProgress, Message: "กำลังดึงข้อมูล..."}
        // ... do work ...
        ch <- sse.Event{Type: sse.EventResult, Data: result}
    }()

    sse.Stream(ctx, sw, ch)
}
```
