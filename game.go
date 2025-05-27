package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	// GameManager
	MAX_GAMES = 10
	// Game
	CANVAS_WIDTH    = 800
	CANVAS_HEIGHT   = 400
	PADDLE_HEIGHT   = 80
	PADDLE_WIDTH    = 10
	BALL_RADIUS     = 5
	PADDLE_SPEED    = 40
	BASE_BALL_SPEED = 10
	MAX_BALL_SPEED  = BASE_BALL_SPEED * 5
	FPS             = 30
)

type gameStatus string

var (
	waiting   gameStatus = "waiting"
	countdown gameStatus = "countdown"
	playing   gameStatus = "playing"
)

type Player struct {
	ID    int     `json:"id"`
	Y     float64 `json:"y"`  // Y position of the paddle
	VY    float64 `json:"vy"` // Vertical speed of the paddle (+/- PADDLE_SPEED)
	Score int     `json:"score"`
	Conn  *websocket.Conn
}

func newPlayer(id int, conn *websocket.Conn) *Player {
	return &Player{
		ID:    id,
		Y:     CANVAS_HEIGHT/2 - PADDLE_HEIGHT/2,
		VY:    0,
		Score: 0,
		Conn:  conn,
	}
}

type Ball struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	VX     float64 `json:"vx"`
	VY     float64 `json:"vy"`
	Radius float64 `json:"radius"`
}

func newBall() Ball {
	return Ball{
		X:      CANVAS_WIDTH / 2,
		Y:      CANVAS_HEIGHT / 2,
		VX:     BASE_BALL_SPEED,
		VY:     BASE_BALL_SPEED,
		Radius: BALL_RADIUS,
	}
}

type Game struct {
	Code        string       `json:"code"`
	LeftPlayer  *Player      `json:"leftPlayer"`
	RightPlayer *Player      `json:"rightPlayer"`
	Ball        Ball         `json:"ball"`
	Status      gameStatus   `json:"status"`
	HitStreak   int          `json:"hitStreak"`
	Countdown   int          `json:"countdown"`
	mutex       sync.Mutex   `json:"-"`
	ticker      *time.Ticker `json:"-"`
	lastUpdate  time.Time    `json:"-"`
}

func newGame(code string) *Game {
	log.Println("new game with code:", code)
	return &Game{
		Code:       code,
		Ball:       newBall(),
		Status:     waiting,
		HitStreak:  0,
		lastUpdate: time.Now(),
	}
}

type GameManager struct {
	games map[string]*Game
	mutex sync.RWMutex
}

func newGameManager() *GameManager {
	return &GameManager{
		games: make(map[string]*Game),
	}
}

func generateGameCode() string {
	return fmt.Sprintf("%05d", rand.Intn(10_000))
}

func (gm *GameManager) CreateGame() *Game {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	if len(gm.games) > MAX_GAMES {
		log.Println("max games reached")
		return nil
	}

	var code string
	for {
		code = generateGameCode()
		if _, exists := gm.games[code]; !exists {
			break
		}
	}

	game := newGame(code)
	gm.games[code] = game
	return game
}

func (gm *GameManager) GetGame(code string) *Game {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()
	return gm.games[code]
}

func (gm *GameManager) RemoveGame(code string) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	if game, exists := gm.games[code]; exists {
		if game.LeftPlayer != nil {
			game.RemovePlayer(1)
		}
		if game.RightPlayer != nil {
			game.RemovePlayer(2)
		}
		if game.ticker != nil {
			game.ticker.Stop()
		}
		delete(gm.games, code)
		log.Printf("game %s removed\n", code)
	} else {
		log.Printf("attempted to remove non-existent game %s\n", code)
	}
}

func (g *Game) AddPlayer(conn *websocket.Conn) int {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	log.Printf("adding player to game %s\n", g.Code)

	var playerID int
	var player *Player

	if g.LeftPlayer == nil {
		playerID = 1
		player = newPlayer(playerID, conn)
		g.LeftPlayer = player
	} else if g.RightPlayer == nil {
		playerID = 2
		player = newPlayer(playerID, conn)
		g.RightPlayer = player
	} else {
		return -1 // Game full
	}

	// Start countdown when both players join
	if g.LeftPlayer != nil && g.RightPlayer != nil {
		g.startCountdown()
	}

	return playerID
}

func (g *Game) RemovePlayer(playerID int) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	log.Printf("removing player %d from game %s\n", playerID, g.Code)

	if playerID == 1 && g.LeftPlayer != nil {
		g.LeftPlayer.Conn.Close(websocket.StatusNormalClosure, "Player disconnected")
		g.LeftPlayer = nil
	} else if playerID == 2 && g.RightPlayer != nil {
		g.RightPlayer.Conn.Close(websocket.StatusNormalClosure, "Player disconnected")
		g.RightPlayer = nil
	}

	g.reset()
}

// Reset the game to the initial state, maintaing the connected players.
// For example, reset the game when 1 player disconnects.
func (g *Game) reset() {
	g.Ball = newBall()
	g.Status = waiting
	g.HitStreak = 0
	g.Countdown = 0

	if g.LeftPlayer != nil {
		g.LeftPlayer.Score = 0
		g.LeftPlayer.VY = 0
		g.LeftPlayer.Y = CANVAS_HEIGHT/2 - PADDLE_HEIGHT/2
	}
	if g.RightPlayer != nil {
		g.RightPlayer.Score = 0
		g.RightPlayer.VY = 0
		g.RightPlayer.Y = CANVAS_HEIGHT/2 - PADDLE_HEIGHT/2
	}
}

func (g *Game) startCountdown() {
	log.Println("starting countdown for game:", g.Code)
	g.Status = countdown
	g.Countdown = 5

	go func() {
		for g.Countdown > 0 {
			g.broadcast(GameCountdownMessage(g.Countdown))
			time.Sleep(1 * time.Second)
			g.Countdown--
		}

		g.Status = playing
		g.broadcast(GameStartMessage())
		g.startGameLoop()
	}()
}

func (g *Game) startGameLoop() {
	log.Println("starting loop for game:", g.Code)
	g.ticker = time.NewTicker(time.Second / FPS)

	go func() {
		defer g.ticker.Stop()
		for range g.ticker.C {
			g.update()
			if g.Status != playing {
				log.Printf("stopping loop for game: %s\n", g.Code)
				return
			}
		}
	}()
}

func (g *Game) update() {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	now := time.Now()
	deltaTime := now.Sub(g.lastUpdate).Seconds()
	g.lastUpdate = now

	// Update ball position
	g.Ball.X += g.Ball.VX * deltaTime * FPS
	g.Ball.Y += g.Ball.VY * deltaTime * FPS

	// Ball collision with top/bottom walls
	if g.Ball.Y <= BALL_RADIUS || g.Ball.Y >= CANVAS_HEIGHT-BALL_RADIUS {
		g.Ball.VY = -g.Ball.VY
		g.Ball.Y = math.Max(BALL_RADIUS, math.Min(CANVAS_HEIGHT-BALL_RADIUS, g.Ball.Y))
	}

	// Update paddle positions
	if g.LeftPlayer != nil {
		Y := g.LeftPlayer.Y + g.LeftPlayer.VY*deltaTime*FPS
		g.LeftPlayer.Y = math.Max(0, math.Min(CANVAS_HEIGHT-PADDLE_HEIGHT, Y))
	}
	if g.RightPlayer != nil {
		Y := g.RightPlayer.Y + g.RightPlayer.VY*deltaTime*FPS
		g.RightPlayer.Y = math.Max(0, math.Min(CANVAS_HEIGHT-PADDLE_HEIGHT, Y))
	}

	// Ball collision with paddles
	if g.LeftPlayer != nil && g.RightPlayer != nil {
		// Left paddle collision
		if g.Ball.X <= PADDLE_WIDTH+BALL_RADIUS &&
			g.Ball.Y >= g.LeftPlayer.Y && g.Ball.Y <= g.LeftPlayer.Y+PADDLE_HEIGHT {
			g.Ball.VX = math.Abs(g.Ball.VX)
			g.Ball.X = PADDLE_WIDTH + BALL_RADIUS
			g.HitStreak++
			g.increaseBallSpeed()
		}

		// Right paddle collision
		if g.Ball.X >= CANVAS_WIDTH-PADDLE_WIDTH-BALL_RADIUS &&
			g.Ball.Y >= g.RightPlayer.Y && g.Ball.Y <= g.RightPlayer.Y+PADDLE_HEIGHT {
			g.Ball.VX = -math.Abs(g.Ball.VX)
			g.Ball.X = CANVAS_WIDTH - PADDLE_WIDTH - BALL_RADIUS
			g.HitStreak++
			g.increaseBallSpeed()
		}
	}

	// Ball out of bounds (scoring)
	if g.Ball.X < 0 {
		if g.RightPlayer != nil {
			g.RightPlayer.Score++
		}
		g.resetBall(1) // Ball goes to left player
	} else if g.Ball.X > CANVAS_WIDTH {
		if g.LeftPlayer != nil {
			g.LeftPlayer.Score++
		}
		g.resetBall(-1) // Ball goes to right player
	}

	g.broadcast(GameStateMessage(g))
}

func (g *Game) increaseBallSpeed() {
	speedMultiplier := 1.0 + float64(g.HitStreak)*0.1

	currentSpeed := math.Sqrt(g.Ball.VX*g.Ball.VX + g.Ball.VY*g.Ball.VY)
	newSpeed := math.Min(BASE_BALL_SPEED*speedMultiplier, MAX_BALL_SPEED)

	if currentSpeed > 0 {
		g.Ball.VX = (g.Ball.VX / currentSpeed) * newSpeed
		g.Ball.VY = (g.Ball.VY / currentSpeed) * newSpeed
	}
}

func (g *Game) resetBall(direction int) {
	g.Ball.X = CANVAS_WIDTH / 2
	g.Ball.Y = CANVAS_HEIGHT / 2
	g.Ball.VX = BASE_BALL_SPEED * float64(direction)
	g.Ball.VY = BASE_BALL_SPEED * (rand.Float64()*2 - 1)
	g.HitStreak = 0
}

func (g *Game) broadcast(msg Message) {
	var wg sync.WaitGroup

	if g.LeftPlayer != nil && g.LeftPlayer.Conn != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			broadcastPlayer(g.LeftPlayer, msg)
		}()
	}

	if g.RightPlayer != nil && g.RightPlayer.Conn != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			broadcastPlayer(g.RightPlayer, msg)
		}()
	}

	wg.Wait()
}

func broadcastPlayer(player *Player, msg Message) {
	if player == nil || player.Conn == nil {
		log.Println("cannot broadcast to nil player or nil connection")
		return
	}
	err := writeJSON(player.Conn, msg)
	if err != nil {
		log.Printf("broadcast error: %v\n", err)
	}
}

// Helper functions for JSON WebSocket communication
func writeJSON(conn *websocket.Conn, v any) error {
	if conn == nil {
		return errors.New("cannot write to nil connection")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return wsjson.Write(ctx, conn, v)
}

func readJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	if conn == nil {
		return errors.New("cannot read from nil connection")
	}
	return wsjson.Read(ctx, conn, v)
}

func (g *Game) movePlayer(player *Player, direction string) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	if player == nil {
		log.Println("cannot move nil player")
		return
	}

	switch direction {
	case "up":
		player.VY = -PADDLE_SPEED
	case "down":
		player.VY = PADDLE_SPEED
	case "stopped":
		player.VY = 0
	default:
		log.Printf("cannot move player direction: '%s'\n", direction)
	}
}
