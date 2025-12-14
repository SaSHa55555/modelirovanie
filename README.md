# ⭐️🩰️Oil Company Model - AnyLogic Wrapper

Go backend wrapper for AnyLogic oil company simulation model with web frontend. 

## Quick Start

```bash
# 1. Start PostgreSQL
brew services start postgresql@15
# Or if first time:
# createdb AnyLogicDB
# psql -d AnyLogicDB -c "CREATE USER postgres WITH SUPERUSER PASSWORD 'postgres';"

# 2. Compile Java runner (one time)
cd model
javac -cp "model.jar:lib/*:lib/logging/*:lib/database/*:lib/database/querydsl/*" ModelRunner.java

# 3. Start server
cd ../backend
go run main.go

# 4. Open browser
open http://localhost:8080
```

## API Endpoints

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| POST | `/api/login` | No | Login with username/password |
| POST | `/api/register` | No | Register new user |
| POST | `/api/logout` | Yes | Logout current session |
| POST | `/api/run-model` | Yes | Run simulation with parameters |
| GET | `/api/history` | Yes | Get user's request history |
| GET | `/api/status` | No | Server status |

## Model Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `scenario` | int (1-3) | Investment strategy |
| `drillingRate` | int | New wells per year |
| `oilPrice` | float | Oil price ($/barrel) |
| `exchangeRate` | float | RUB/USD rate |

## Default Users

- `admin` / `admin123`
- `user` / `user123`

New users can register via UI (stored in memory, lost on restart).

## Project Structure

```
modelirovanie/
├── backend/
│   └── main.go          # Go HTTP server
├── frontend/
│   └── index.html       # Web UI
├── model/
│   ├── model.jar        # AnyLogic model
│   ├── ModelRunner.java # Java wrapper
│   └── lib/             # Dependencies
└── README.md
```

## Features

- Authentication with session tokens
- Request history logging to PostgreSQL
- Interactive charts (Revenue, Production, Wells)
