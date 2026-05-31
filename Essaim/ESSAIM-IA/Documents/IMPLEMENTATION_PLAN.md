# PLAN D'IMPLÉMENTATION AUTOMATISÉ - ESSAIM IA

## RÈGLE POUR L'AGENT (TOI) :

1. Lis ce fichier.
2. Trouve la première tâche non cochée `[ ]`.
3. Exécute-la ENTIÈREMENT (crée les fichiers, écris le code).
4. Coche la case `[x]` dans ce fichier une fois terminée et vérifiée.
5. Passe IMMÉDIATEMENT à la tâche suivante sans demander l'avis de l'utilisateur.
6. Si une étape échoue, corrige-la avant de passer à la suivante.

---

## PHASE 1 : Backend (Core & Domain)

- [x] **1. Setup Domain Types**

  - Créer `/backend/internal/domain/agent/entity.go` (Struct Agent + Tags JSON).
  - Créer `/backend/internal/domain/message/types.go` (Struct Packet, Header, Body).
  - Créer `/backend/internal/domain/graph/node.go`.
  - *Vérification :* Le code doit compiler (`go build ./...`).
- [x] **2. Core Logic (Dispatcher, Routing & Economy)**

  - Créer `/backend/internal/core/economy/budget.go` (Gestion Tokens).
  - Créer `/backend/internal/core/orchestrator/dispatcher.go` (Worker Pool, Channels).
  - Créer `/backend/internal/core/routing/bubble.go` (Gestion du Postier et des Résumeurs dans la bulle).
  - Implémenter le "Kill Switch" si budget <= 0.

## PHASE 2 : Infrastructure

- [x] **3. Persistence Layer**

  - Implémenter `/backend/internal/infrastructure/persistence/mongo_repo.go`.
  - Doit utiliser le driver officiel mongo-go.
- [x] **4. LLM Client**

  - Implémenter `/backend/internal/infrastructure/llm/client.go`.
  - Intégrer Gemini API Client.
  - Ajouter le Rate Limiter (Token Bucket) pour gérer les quotas.
- [x] **5. Server Assembly**

  - Créer `/backend/cmd/server/main.go`.
  - Initialiser DB, Dispatcher, WebSocket Hub.
  - Lancer le serveur HTTP.

## PHASE 3 : Frontend & Connection

- [x] **6. React Components**
  - Configurer `react-force-graph-2d` dans `/frontend`.
  - Créer `NetworkGraph.jsx` avec le code couleur (Bleu=Architecte, Vert=Worker).
  - Créer le Hook `useSocket.js` pour écouter le port 8080.

## PHASE 4 : Intelligence

- [x] **7. System Prompts**
  - Dans `/backend/internal/domain/agent`, implémenter la génération dynamique du Prompt.
  - Injecter : ID, Rôle, BubbleID, Budget.
  - Créer les templates spécifiques : Architecte, Postier, Résumeur, Worker, Critique.
  - Forcer le format JSON STRICT dans le prompt système.

## PHASE 5 : Finalisation

- [x] **8. Genesis Script**
  - Créer une route API POST `/start` qui injecte le premier Agent "Alpha".
