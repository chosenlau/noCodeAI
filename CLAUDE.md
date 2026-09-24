# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a no-code AI platform that generates code (HTML/multi-file applications) from natural language prompts using LLM agents (Claude/OpenAI). Built with Go, Hertz web framework, and CloudWeGo Eino for AI orchestration.

**Core workflow**: User submits prompt → AI agent generates structured code → Parser validates → Saver writes to filesystem → App deployed

## Build & Run Commands

```bash
# Start the server (reads config-local.yml by default)
go run cmd/main/main.go

# Start with specific environment
go run cmd/main/main.go -env=dev    # Uses config/config-dev.yml
go run cmd/main/main.go -env=test   # Uses config/config-test.yml

# Generate GORM DAL code from database schema
go run cmd/gen/main.go

# Run Wire dependency injection code generation
cd wire && wire
```

## Architecture

### Dependency Injection (Wire)

This project uses Google Wire for compile-time dependency injection. The wiring is defined in `wire/wire.go`:
- `configSet`: Configuration initialization
- `dbSet`: Database connection
- `serviceSet`: Business services
- `handlerSet`: HTTP handlers

After modifying wire definitions, regenerate with `cd wire && wire` to update `wire_gen.go`.

### Layer Structure

```
Handler (HTTP) → Service (Business Logic) → Logic (Domain Logic) → DAL (Data Access)
                                          ↘ AI Components (Agent/Model/Prompt)
```

- **Handler**: HTTP request/response handling, authentication via cookies
- **Service**: Orchestrates logic and DAL operations
- **Logic**: Core business rules and validation
- **DAL**: GORM-generated query builders (`internal/dal/query/`) and models (`internal/dal/model/`)
- **AI Components**:
  - `internal/ai/agent/`: CodeGenAgent with HTML/MultiFile generation capabilities
  - `internal/ai/llm/`: LLM model wrappers (OpenAI, Claude)
  - `internal/ai/prompt/`: System prompts loaded from files
  - `internal/core/`: AICodeGenFacade orchestrates generation + streaming + saving

### Code Generation Flow

1. **Handler** receives SSE request at `/app/chat` (ChatToGenCode)
2. **Service** retrieves app metadata, calls AICodeGenFacade
3. **Facade** (`ai_codegen_facade.go`):
   - Calls appropriate agent (HTML or MultiFile)
   - Copies stream: one for client response, one for background processing
   - Background goroutine parses JSON response and saves files
4. **Parser** (`core/parser/`) validates generated code structure
5. **Saver** (`core/saver/`) writes files to filesystem under generated app directory

### Database Layer (GORM Gen)

Models and queries are **code-generated** from the database schema:
- Models: `internal/dal/model/*.gen.go` (App, User)
- Queries: `internal/dal/query/*.gen.go` (type-safe query builders)

**After schema changes**: Run `go run cmd/gen/main.go` to regenerate DAL code.

### Authentication

Cookie-based authentication using `user_id` cookie:
- Set on login, checked via `middleware.AuthMiddleware()`
- Admin routes require additional `middleware.DevRequireAdmin()`
- User role enum: `pkg/enum/user_role.go` (Admin, User, Ban)

## Configuration

Environment-specific YAML files in `config/`:
- `config-local.yml`: Local development (default)
- `config-dev.yml`, `config-test.yml`: Other environments

Config structure (`config/config.go`):
```go
type Config struct {
    Server   ServerConfig   // Port, context path
    Database DatabaseConfig // MySQL connection
    AI       AIConfig       // LLM API key, model, base URL
}
```

Pass `-env=<name>` flag to select config file.

## AI Model Configuration

The system supports multiple LLM providers via CloudWeGo Eino:
- **OpenAI-compatible**: `internal/ai/llm/openai_base_model.go`
- **Claude**: `internal/ai/llm/claude_base_model.go`

Model is selected via config file's `ai.model` field. Both implement `ChatModelWrapperAdaptor` interface for agent compatibility.

## Code Generation Types

Defined in `pkg/enum/code_gentype.go`:
- `HtmlCodeGen`: Single HTML file with inline CSS/JS
- `MultiFileGen`: Multi-file project structure (HTML, CSS, JS, assets)

Each type has dedicated:
- Agent method (`GenerateHtmlCodeStream`, `GenerateMultiFileCodeStream`)
- Prompt template (`internal/ai/prompt/`)
- Response structure (`internal/ai/ai_model/aicode_result.go`)
- Parser (`internal/core/parser/gencode_parser.go`)

## API Routes

Defined in `internal/router/router.go`:

**User routes** (`/user`):
- Public: `/register`, `/login`, `/get/vo`
- Authenticated: `/get/login`, `/logout`
- Admin: `/add`, `/update`, `/delete`, `/list/page/vo`

**App routes** (`/app`):
- Public: `/good/list/page/vo` (featured apps)
- Authenticated: `/my/list/page/vo`, `/add`, `/update`, `/delete`, `/get/vo`
- Admin: `/admin/update`, `/admin/delete`, `/admin/get/vo`, `/admin/list/page/vo`
- Code generation (SSE): `/chat` (not yet registered in router, implement via `ChatToGenCode` handler)

## Database

MySQL database with tables:
- `user`: User accounts with role-based access
- `app`: Generated applications with metadata (name, prompt, code type, deploy key)

Connection initialized via `dal.InitDB(cfg)` using DSN from config.

## Key Patterns

1. **Streaming responses**: AI generation uses SSE (Server-Sent Events) for real-time streaming
2. **Dual stream processing**: Facade copies stream to return to client while processing in background
3. **JSON response parsing**: Agent responses are JSON wrapped in markdown code blocks (```json)
4. **Error handling**: Custom error types in `pkg/errorutil/business_error.go`
5. **Response wrapping**: All API responses use `pkg/response/base_response.go` structure
6. **Snowflake IDs**: Custom ID generation using Sony Snowflake (`pkg/snowflake/`)

## Important Notes

- Wire-generated code (`wire_gen.go`) should not be manually edited
- GORM-generated files (`*.gen.go`) are regenerated from database schema
- AI prompts are loaded from files at runtime via `prompt.LoadPrompts()`
- Stream processing happens asynchronously - client receives data while files are being saved
- All handlers return HTTP 200 with error details in response body (not status codes)
