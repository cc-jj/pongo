package main

type MessageType string

const (
	MessageTypePlayerAssigned MessageType = "playerAssigned"
	MessageTypeGameCountdown  MessageType = "countdown"
	MessageTypeGameStart      MessageType = "gameStart"
	MessageTypeGameState      MessageType = "gameState"
	MessageTypeMove           MessageType = "move"
)

type Message struct {
	Type MessageType `json:"type"`
	Data any         `json:"data"`
}

func PlayerAssignedMessage(playerID int, game *Game) Message {
	return Message{
		Type: MessageTypePlayerAssigned,
		Data: map[string]any{
			"playerId":  playerID,
			"gameState": game,
		},
	}
}

func GameCountdownMessage(countdown int) Message {
	return Message{
		Type: MessageTypeGameCountdown,
		Data: countdown,
	}
}

func GameStartMessage() Message {
	return Message{
		Type: MessageTypeGameStart,
		Data: nil,
	}
}

func GameStateMessage(game *Game) Message {
	return Message{
		Type: MessageTypeGameState,
		Data: game,
	}
}

type MoveMessage struct {
	Type MessageType `json:"type"` // "move"
	Data string      `json:"data"` // "up", "down", "stopped"
}
