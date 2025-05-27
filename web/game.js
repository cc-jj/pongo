const UP_KEYS = new Set(["w", "arrowup"]);
const DOWN_KEYS = new Set(["s", "arrowdown"]);

class PongGame {
  constructor() {
    this.canvas = document.getElementById("gameCanvas");
    this.ctx = this.canvas.getContext("2d");
    this.ws = null;
    this.playerId = null;
    this.gameState = null;
    this.lastDirection = "stopped";
    this.animationFrameId = null;

    this.initializeGame();
    this.setupEventListeners();
    this.connectWebSocket();
  }

  initializeGame() {
    // Get game code from URL
    const urlParams = new URLSearchParams(window.location.search);
    this.gameCode = urlParams.get("code");
    document.getElementById("gameCodeDisplay").textContent =
      this.gameCode || "------";

    if (!this.gameCode) {
      this.showError("No game code provided");
      return;
    }
  }

  connectWebSocket() {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${protocol}//${window.location.host}/ws/${this.gameCode}`;

    this.ws = new WebSocket(wsUrl);

    this.ws.onopen = () => {
      console.log("Connected to game server");
      this.updateConnectionStatus(true);
    };

    this.ws.onmessage = (event) => {
      const message = JSON.parse(event.data);
      this.handleMessage(message);
    };

    this.ws.onclose = () => {
      console.log("Disconnected from game server");
      this.updateConnectionStatus(false);
    };

    this.ws.onerror = (error) => {
      console.error("WebSocket error:", error);
      this.updateConnectionStatus(false);
    };
  }

  handleMessage(message) {
    switch (message.type) {
      case "playerAssigned":
        this.playerId = message.data.playerId;
        this.gameState = message.data.gameState;
        this.updateUI();
        break;

      case "gameState":
        this.gameState = message.data;
        this.updateUI();
        this.animationFrameId = requestAnimationFrame(() => this.draw());
        break;

      case "countdown":
        this.showCountdown(message.data);
        break;

      case "gameStart":
        this.hideOverlay();
        break;

      case "error":
        this.showError(message.data);
        break;

      default:
        console.error("Unknown message:", message);
        break;
    }
  }

  updateUI() {
    if (!this.gameState) return;

    // Update scores
    const leftScore = this.gameState.leftPlayer
      ? this.gameState.leftPlayer.score
      : 0;
    const rightScore = this.gameState.rightPlayer
      ? this.gameState.rightPlayer.score
      : 0;

    document.getElementById("player1Score").textContent = leftScore;
    document.getElementById("player2Score").textContent = rightScore;

    // Update hit streak
    const hitStreakEl = document.getElementById("hitStreak");
    const hitStreakValue = document.getElementById("hitStreakValue");

    if (this.gameState.hitStreak > 0) {
      hitStreakEl.style.display = "block";
      hitStreakValue.textContent = this.gameState.hitStreak;
    } else {
      hitStreakEl.style.display = "none";
    }

    // Update game state
    if (this.gameState.status === "waiting") {
      const playerCount =
        (this.gameState.leftPlayer ? 1 : 0) +
        (this.gameState.rightPlayer ? 1 : 0);
      if (playerCount === 1) {
        this.showWaiting("Waiting for second player...");
      }
    } else if (this.gameState.status === "playing") {
      this.hideOverlay();
    }
  }

  draw() {
    if (!this.gameState) return;

    const ctx = this.ctx;
    const canvas = this.canvas;

    // Clear canvas
    ctx.fillStyle = "#000";
    ctx.fillRect(0, 0, canvas.width, canvas.height);

    // Draw center line
    ctx.strokeStyle = "#fff";
    ctx.lineWidth = 2;
    ctx.setLineDash([10, 10]);
    ctx.beginPath();
    ctx.moveTo(canvas.width / 2, 0);
    ctx.lineTo(canvas.width / 2, canvas.height);
    ctx.stroke();
    ctx.setLineDash([]);

    if (this.gameState.status !== "playing") {
      return;
    }

    // Draw paddles
    ctx.fillStyle = "#fff";

    if (this.gameState.leftPlayer) {
      ctx.fillRect(0, this.gameState.leftPlayer.y, 10, 80);
    }

    if (this.gameState.rightPlayer) {
      ctx.fillRect(canvas.width - 10, this.gameState.rightPlayer.y, 10, 80);
    }

    // Draw ball as a circle
    if (this.gameState.ball) {
      ctx.fillStyle = "#fff";
      ctx.beginPath();
      ctx.arc(
        this.gameState.ball.x,
        this.gameState.ball.y,
        this.gameState.ball.radius,
        0,
        2 * Math.PI
      );
      ctx.fill();
    }

    // Highlight player's paddle
    if (this.playerId === 1 && this.gameState.leftPlayer) {
      ctx.strokeStyle = "#00ff4c";
      ctx.lineWidth = 3;
      ctx.strokeRect(-1, this.gameState.leftPlayer.y - 2, 12, 84);
    } else if (this.playerId === 2 && this.gameState.rightPlayer) {
      ctx.strokeStyle = "#00ff4c";
      ctx.lineWidth = 3;
      ctx.strokeRect(
        canvas.width - 11,
        this.gameState.rightPlayer.y - 2,
        12,
        84
      );
    }
  }

  showCountdown(count) {
    const overlay = document.getElementById("statusOverlay");
    overlay.innerHTML = `<div class="countdown">${count}</div>`;
    overlay.style.display = "block";
  }

  showWaiting(message) {
    const overlay = document.getElementById("statusOverlay");
    overlay.innerHTML = `
                    <div class="waiting-message">${message}</div>
                    <div>Share your game code with a friend!</div>
                `;
    overlay.style.display = "block";
  }

  hideOverlay() {
    document.getElementById("statusOverlay").style.display = "none";
  }

  showError(message) {
    const overlay = document.getElementById("statusOverlay");
    overlay.innerHTML = `
                    <div class="waiting-message">Error</div>
                    <div>${message}</div>
                    <div style="margin-top: 15px;">
                        <a href="/" class="back-button">← Back to Menu</a>
                    </div>
                `;
    overlay.style.display = "block";
  }

  updateConnectionStatus(connected) {
    const status = document.getElementById("connectionStatus");
    if (connected) {
      status.textContent = "Connected";
      status.className = "connection-status connected";
    } else {
      status.textContent = "Disconnected";
      status.className = "connection-status disconnected";
    }
  }

  setupEventListeners() {
    // Keyboard controls
    document.addEventListener("keydown", (e) => this.handleKeyDown(e));
    document.addEventListener("keyup", (e) => this.handleKeyUp(e));
  }

  handleKeyDown(e) {
    const key = e.key.toLowerCase();
    if (UP_KEYS.has(key)) {
      this.handleMove("up");
    } else if (DOWN_KEYS.has(key)) {
      this.handleMove("down");
    }
  }

  handleKeyUp(e) {
    const key = e.key.toLowerCase();
    if (UP_KEYS.has(key) || DOWN_KEYS.has(key)) {
      this.handleMove("stopped");
    }
  }

  handleMove(direction) {
    if (direction === this.lastDirection) {
      return; // No change in direction
    }
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(
        JSON.stringify({
          type: "move",
          data: direction,
        })
      );
      this.lastDirection = direction;
    }
  }
}

// Initialize game when page loads
window.addEventListener("load", () => {
  new PongGame();
});
