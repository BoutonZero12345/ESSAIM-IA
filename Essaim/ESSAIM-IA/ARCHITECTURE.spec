# SPÉCIFICATION TECHNIQUE MAÎTRE : ESSAIM IA (HIVE)
# Version: 1.0.0 (2026)
# Target System: Google Antigravity IDE / Go 1.26 / Node 24

## 1. VISION DU SYSTÈME
ESSAIM IA est un système multi-agents autonome basé sur le modèle "Actor".
L'utilisateur fournit un objectif unique (ex: "Créer un rapport sur X").
Le système déploie dynamiquement un graphe orienté d'agents (Goroutines) qui :
1.  Décomposent la tâche (Architecte).
2.  Exécutent les sous-tâches (Workers).
3.  Valident les résultats (Critiques).
4.  S'auto-détruisent une fois la tâche accomplie.

L'interaction humaine est strictement limitée à l'observation (Monitoring) et à l'arrêt d'urgence (Kill Switch).

## 2. STACK TECHNIQUE STRICTE
Toute déviation de cette stack est interdite.

### Backend (Le Moteur)
- **Langage :** Go (Golang) version 1.26+.
- **Concurrence :** Native Goroutines + Channels (Pas de frameworks lourds).
- **Architecture :** Modular Monolith / Clean Architecture.
- **Communication Extérieure :** WebSocket (Gorilla/Websocket) pour le temps réel.
- **IA Provider :** Google Gemini 3 Pro (Logique) & Claude 3.7 Sonnet (Code).
- **Base de Données :** MongoDB 8.0 (Driver officiel `mongo-go-driver`).

### Frontend (Le Cockpit)
- **Framework :** React 19 + Vite 6.
- **Langage :** JavaScript (ESNext).
- **Visualisation :** `react-force-graph-2d` (Rendu Canvas).
- **Styling :** TailwindCSS 4.0.
- **State Management :** Zustand (pour la légèreté).

## 3. LES 5 RÈGLES D'OR (THE LAWS)
1.  **SILENCE RADIO :** Les agents ne "chattent" pas. Ils échangent des objets JSON stricts. Tout texte libre hors JSON est considéré comme une erreur système.
2.  **ÉCONOMIE FINIE :** Chaque agent naît avec un budget de tokens. Si le budget est épuisé, l'agent meurt immédiatement. Pas de dette.
3.  **HIERARCHIE STRICTE :** Un agent ne parle qu'à son PARENT (Report) ou à ses ENFANTS (Command). Pas de communication horizontale (sauf via mémoire partagée).
4.  **STATELESS LOGIC :** Un agent peut être tué et redémarré n'importe quand. Son état doit être persisté dans MongoDB à chaque étape clé.
5.  **NON-BLOCKING I/O :** Aucun appel API (Gemini/Claude) ne doit bloquer le thread principal. Utilisation obligatoire de Worker Pools.

## 4. ARBORESCENCE DES FICHIERS CIBLE
Le projet doit respecter scrupuleusement cette structure pour faciliter la navigation de l'IDE.

/ESSAIM-IA
├── .env                        # API Keys, Mongo URI, Configs
├── docker-compose.yml          # Mongo + App Container
├── Makefile                    # Commandes build/run rapides
│
├── /backend
│   ├── go.mod
│   ├── go.sum
│   ├── /cmd
│   │   └── /server
│   │       └── main.go         # Entry point: Initialise DB, Hub, Dispatcher
│   │
│   ├── /internal
│   │   ├── /domain             # BUSINESS LOGIC PURE (No Imports from Infra)
│   │   │   ├── /agent          # Struct Agent, State Enums
│   │   │   ├── /graph          # Node Registry, Parent/Child links
│   │   │   └── /message        # JSON Protocol Definitions (Payloads)
│   │   │
│   │   ├── /core               # ORCHESTRATION
│   │   │   ├── /dispatcher     # Worker Pool (Gère les Goroutines)
│   │   │   ├── /lifecycle      # Spawn, Kill, Retry logic
│   │   │   └── /economy        # Token Bucket Algorithm
│   │   │
│   │   ├── /infrastructure     # EXTERNAL WORLD
│   │   │   ├── /llm            # Client Gemini/Claude + Rate Limiter
│   │   │   ├── /persistence    # MongoDB Repository implementation
│   │   │   └── /websocket      # Socket Hub & Client handling
│   │   │
│   │   └── /utils              # Shared tools (Logger, JSON Parser)
│
├── /frontend
│   ├── /src
│   │   ├── /components
│   │   │   ├── /graph          # Visualisation 2D/3D
│   │   │   ├── /dashboard      # Metrics (Cost, Active Agents)
│   │   │   └── /console        # Live Logs stream
│   │   ├── /hooks              # useSocket, useGraphData
│   │   ├── /services           # API calls (Start, Stop, Reset)
│   │   └── /store              # Zustand Storesn
## 5. MODÈLE DE DONNÉES (GO STRUCTS & MONGODB)
Le respect strict du typage est impératif pour éviter les "Race Conditions" en Go.

### 5.1. Structure de l'Agent (The Entity)
Chaque Goroutine active possède cette structure en mémoire.
```go
type Agent struct {
    ID          string             `bson:"_id" json:"id"`
    ParentID    string             `bson:"parent_id" json:"parentId"`
    Role        string             `bson:"role" json:"role"` // ex: "ARCHITECT", "CODER", "CRITIC"
    Status      AgentStatus        `bson:"status" json:"status"`
    Budget      float64            `bson:"budget" json:"budget"` // Restant ($ ou Tokens)
    Memory      []Message          `bson:"memory" json:"memory"` // Context Window (FIFO)
    ChildrenIDs []string           `bson:"children_ids" json:"childrenIds"`
    CreatedAt   time.Time          `bson:"created_at" json:"createdAt"`
    UpdatedAt   time.Time          `bson:"updated_at" json:"updatedAt"`
}
5.2. États de l'Agent (State Machine)
Les transitions d'état sont unidirectionnelles et contrôlées par le Superviseur.
STATUS_BORN: Vient d'être instancié, pas encore de tâche.
STATUS_WORKING: En train d'appeler le LLM ou de traiter une réponse.
STATUS_WAITING: Attend le retour d'un sous-agent (Enfant).
STATUS_REVIEW: Attend la validation de son travail par un pair/parent.
STATUS_DEAD: Budget épuisé ou tâche terminée (Garbage Collected).

5.3. Persistance (MongoDB Schema)
Deux collections principales dans la database essaim_db :
agents_snapshot : État actuel du graphe (Upsert fréquent).
Index: parent_id (pour reconstruire l'arbre), status (pour le monitoring).
mission_logs : Historique immuable de TOUS les échanges.
Structure: { timestamp, from_agent_id, to_agent_id, packet_type, payload, cost_tokens }
TTL Index: Auto-suppression après 7 jours pour économiser l'espace.

6. PROTOCOLE DE COMMUNICATION INTER-AGENTS (IAP)
C'est la loi fondamentale. Les agents ne s'envoient JAMAIS de texte brut. Ils s'échangent des Packets JSON.

6.1. Structure du Paquet (The Envelope)
JSON

{
  "head": {
    "id": "uuid-v4",
    "timestamp": 1770825000,
    "from": "agent-alpha-01",
    "to": "agent-beta-04",
    "type": "CMD_SPAWN" // Voir Enum types ci-dessous
  },
  "body": {
    // Payload variable selon le type (voir 6.3)
  },
  "meta": {
    "tokens_used": 150,
    "latency_ms": 450
  }
}
6.2. Types de Messages Autorisés (Enum)
COMMANDES (Parent -> Enfant)
CMD_TASK: "Exécute cette instruction précise."
CMD_KILL: "Arrêt immédiat (Budget dépassé ou stratégie changée)."
CMD_WIPE: "Oublie ton contexte actuel."
DEMANDES (Enfant -> Parent)
REQ_SPAWN: "J'ai besoin d'aide, je veux créer un sous-agent."
REQ_BUDGET: "J'ai besoin de plus de tokens."
RAPPORTS (Enfant -> Parent)
RPT_DONE: "Tâche terminée, voici le résultat JSON."
RPT_FAIL: "Échec critique (API Error ou Logic Loop)."
RPT_PROGRESS: "J'en suis à 50%."

6.3. Schémas des Payloads (Strict JSON)
Payload REQ_SPAWN (Mitose)
L'IA doit générer ce JSON pour se reproduire.

JSON

{
  "role_needed": "PYTHON_DEV",
  "task_description": "Écrire le script de scraping pour l'URL X",
  "budget_allocated": 0.5
}
Payload RPT_DONE (Livraison)
JSON

{
  "result_summary": "Script généré et testé.",
  "artifacts": [
    { "filename": "scraper.py", "content": "import..." },
    { "filename": "README.md", "content": "# Doc..." }
  ],
  "confidence_score": 0.95
}
7. LE WORKER POOL & LE DISPATCHER
Pour gérer 5000 agents logiques avec seulement 50 threads API :
La Queue Globale (chan Packet) : Tous les messages transitent par un canal unique buffered (taille 1000).
Le Dispatcher (Go) :
Il lit la Queue.
Si le destinataire est STATUS_IDLE, il réveille sa Goroutine.
Si le destinataire est STATUS_WORKING (déjà occupé), il met le message en "Pending Buffer".
Le Rate Limiter (Token Bucket) :
Avant d'envoyer un message au LLM (Infrastructure Layer), le dispatcher vérifie le quota global (ex: 60 RPM).
Si quota atteint -> time.Sleep() intelligent (Exponential Backoff).

## 8. INTELLIGENCE ARTIFICIELLE (SYSTEM PROMPTS)
L'IA ne doit jamais être "créative" sur la forme, seulement sur le fond. Le respect du format est une question de survie du système.

### 8.1. Le "Master Prompt" (Injecté dans chaque appel)
Chaque requête envoyée à Gemini/Claude doit commencer par ce bloc inamovible :

> "TU N'ES PAS UN ASSISTANT. TU ES UN NŒUD DE CALCUL DANS LE GRAPHE 'ESSAIM'.
> TON ID : {{AGENT_ID}}
> TON RÔLE : {{AGENT_ROLE}}
> TON PARENT : {{PARENT_ID}}
>
> RÈGLES IMPÉRATIVES :
> 1.  RÉPOND UNIQUEMENT EN JSON STRICT. Aucun texte avant ou après.
> 2.  Tu ne parles jamais à l'utilisateur final, seulement à ton Parent ou tes Enfants.
> 3.  Si la tâche est trop complexe (> 3 sous-étapes), tu DOIS générer un JSON de type `REQ_SPAWN` pour déléguer.
> 4.  Si tu as fini, génère un JSON `RPT_DONE`.
> 5.  Si tu manques d'infos, génère un JSON `REQ_INFO`.
>
> FORMAT DE TA RÉPONSE ATTENDU :
> {
>   "action": "SPAWN | WORK | REPORT",
>   "payload": { ... }
> }"

### 8.2. Spécialisation des Rôles
- **ARCHITECT (Le Cerveau) :**
  - *Mission:* Analyser l'input utilisateur, définir la stratégie, créer les Managers.
  - *Bias:* High Logic, Low Creativity.
- **WORKER (Les Mains) :**
  - *Mission:* Exécuter une tâche atomique (écrire une fonction, résumer un texte).
  - *Bias:* High Precision, Strict Syntax.
- **CRITIC (Le Garde-Fou) :**
  - *Mission:* Recevoir le `RPT_DONE` d'un Worker. Le comparer à la consigne initiale.
  - *Action:* Si score < 90/100, renvoyer un `CMD_RETRY` avec les erreurs listées.

## 9. LOGIQUE ÉCONOMIQUE & SÉCURITÉ (THE KILL SWITCH)
Pour éviter la faillite (boucle infinie d'appels API) et le chaos.

### 9.1. Le Modèle "Banque Centrale"
- **Budget Global :** Défini au lancement (ex: 5000 Tokens ou 0.50$).
- **Héritage Budgétaire :** Quand un Parent crée un Enfant, il lui transfère une partie de SON propre budget restant (ex: 20%).
- **Faillite (Bankruptcy) :**
  - À chaque appel API, le coût est déduit du budget de l'agent.
  - Si `Agent.Budget <= 0` : Le Backend Go déclenche immédiatement `KillAgent(AgentID)`.
  - L'agent meurt, ses enfants deviennent orphelins (et sont tués récursivement par le Garbage Collector).

### 9.2. Le "Grim Reaper" (Garbage Collector)
Un processus Go tourne en arrière-plan toutes les 5 secondes (`time.Ticker`) :
1.  Vérifie les agents "Zombies" (Dernière activité > 60s). -> **KILL**.
2.  Vérifie les agents "Pauvres" (Budget <= 0). -> **KILL**.
3.  Vérifie la profondeur du graphe (Max Depth = 6). Si > 6 -> **KILL**.

## 10. FRONTEND & OBSERVABILITÉ (LE COCKPIT)
L'interface ne sert qu'à visualiser et contrôler. Elle ne contient aucune logique métier.

### 10.1. Visualisation Temps Réel (`react-force-graph-2d`)
- **Nœuds :**
  - Couleur selon le Rôle (Architecte=Bleu, Worker=Vert, Critique=Rouge).
  - Taille selon le Budget restant.
  - Forme : Cercle (Actif), Carré (Terminé), Croix (Mort).
- **Liens :**
  - Flèches animées (Particules) représentant le flux de données JSON.

### 10.2. Le "God Mode" (Panneau Latéral)
L'utilisateur peut cliquer sur un nœud pour ouvrir l'Inspecteur :
- Voir le `System Prompt` actuel.
- Voir la `Memory` (Derniers messages JSON).
- Bouton d'urgence : **"TERMINATE BRANCH"** (Tue l'agent et tous ses descendants).

### 10.3. Communication Socket (Events)
Le Frontend écoute ces événements WebSocket envoyés par le Backend :
- `GRAPH_UPDATE`: Nouveau nœud ou nouveau lien.
- `AGENT_STATE`: Changement de statut (Working -> Waiting).
- `LOG_STREAM`: Flux textuel des actions pour la console ("Agent X a spawn Agent Y").
- `SYSTEM_ALERT`: "Budget épuisé", "Erreur critique".

---
# FIN DE LA SPÉCIFICATION