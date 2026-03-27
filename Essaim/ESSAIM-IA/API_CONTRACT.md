# Contrat API — ESSAIM-IA Backend

Ce document remplace le frontend comme source de vérité pour les routes HTTP et WebSocket exposées par le serveur Go.

## Endpoints HTTP

### POST /start
Lance une nouvelle mission.

**Corps de la requête (JSON) :**
```json
{
  "objective": "string — description de la mission",
  "budget": 50000
}
```

**Réponse :**
```json
{
  "agent_id": "uuid de l'agent racine",
  "status": "started"
}
```

---

### GET /health
Vérification de santé du serveur.

**Réponse :**
```json
{
  "status": "ok"
}
```

---

### GET /ws
Connexion WebSocket pour recevoir les événements temps réel du système.

**URL :** `ws://localhost:8080/ws`

## Événements WebSocket (Backend → Client)

### GRAPH_UPDATE
Émis quand un agent est ajouté ou supprimé.

```json
{
  "type": "GRAPH_UPDATE",
  "payload": {
    "action": "ADD_NODE" | "REMOVE_NODE",
    "agent": { ... },
    "agentId": "uuid"
  }
}
```

### AGENT_STATE
Émis quand le statut d'un agent change.

```json
{
  "type": "AGENT_STATE",
  "payload": {
    "agentId": "uuid",
    "status": "STATUS_WORKING | STATUS_WAITING | STATUS_DEAD",
    "budget": 1234.56,
    "role": "WORKER | POSTIER | ARCHITECT | ..."
  }
}
```

### LOG_STREAM
Flux de logs en temps réel.

```json
{
  "type": "LOG_STREAM",
  "payload": {
    "message": "texte du log"
  }
}
```

### SYSTEM_ALERT
Alerte système ou résultat final de mission.

```json
{
  "type": "SYSTEM_ALERT",
  "payload": {
    "message": "Mission terminée par agent-XX",
    "result": { ... }
  }
}
```

## Test rapide avec curl

```powershell
# Lancer une mission
curl -X POST http://localhost:8080/start `
  -H "Content-Type: application/json" `
  -d '{"objective": "Rédige un poème en 4 strophes sur la mer", "budget": 10000}'

# Santé du serveur
curl http://localhost:8080/health
```
