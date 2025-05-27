package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

var (
	// Only allow websocket connections from these origins
	WS_ORIGINS = []string{"localhost"}
)

type Server struct {
	gameManager *GameManager
}

func (s *Server) indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "web/index.html")
}

func (s *Server) newGameHandler(w http.ResponseWriter, r *http.Request) {
	game := s.gameManager.CreateGame()
	if game == nil {
		http.Error(w, "Failed to create game", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/game?code=%s", game.Code), http.StatusSeeOther)
}

func (s *Server) gameHandler(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Game code required", http.StatusBadRequest)
		return
	}

	game := s.gameManager.GetGame(code)
	if game == nil {
		http.Error(w, "Game not found", http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, "web/game.html")
}

func (s *Server) wsHandler(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" {
		http.Error(w, "Game code required", http.StatusBadRequest)
		return
	}

	game := s.gameManager.GetGame(code)
	if game == nil {
		http.Error(w, "Game not found", http.StatusNotFound)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: WS_ORIGINS,
	})
	if err != nil {
		log.Printf("WebSocket accept error: %v", err)
		http.Error(w, "Failed to connect", http.StatusInternalServerError)
		return
	}
	defer conn.CloseNow()

	playerID := game.AddPlayer(conn)
	if playerID == -1 {
		conn.Close(websocket.StatusUnsupportedData, "Game is full")
		return
	}

	// Send initial game state
	err = writeJSON(conn, PlayerAssignedMessage(playerID, game))
	if err != nil {
		log.Printf("Error sending initial message: %v", err)
		return
	}

	// Handle messages from client
	ctx := context.Background()
	for {
		var msg Message
		err := readJSON(ctx, conn, &msg)
		if err != nil {
			closeStatus := websocket.CloseStatus(err)
			if closeStatus != websocket.StatusGoingAway {
				log.Printf("WebSocket read error: %v", err)
			}
			game.RemovePlayer(playerID)
			if game.LeftPlayer == nil && game.RightPlayer == nil {
				s.gameManager.RemoveGame(code)
			}
			break
		}

		switch msg.Type {
		case "move":
			if direction, ok := msg.Data.(string); ok {
				var player *Player
				if playerID == 1 {
					player = game.LeftPlayer
				} else if playerID == 2 {
					player = game.RightPlayer
				}
				game.movePlayer(player, direction)
			} else {
				log.Printf("Client sent invalid move message: %v", msg.Data)
			}
		default:
			log.Printf("Client sent unexpected message type: %s", msg.Type)
		}
	}
}

func main() {
	rand.New(rand.NewSource(time.Now().UnixNano()))

	server := &Server{
		gameManager: newGameManager(),
	}

	mux := http.NewServeMux()

	fsys := http.FileServer(http.Dir("web"))
	mux.Handle("/web/", http.StripPrefix("/web/", fsys))

	mux.HandleFunc("/", server.indexHandler)
	mux.HandleFunc("/game/new", server.newGameHandler)
	mux.HandleFunc("/game", server.gameHandler)
	mux.HandleFunc("/ws/{code}", server.wsHandler)

	fmt.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
