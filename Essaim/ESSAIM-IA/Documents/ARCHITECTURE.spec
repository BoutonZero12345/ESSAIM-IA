# SPÉCIFICATION TECHNIQUE MAÎTRE : ESSAIM IA (HIVE)
# Version: 1.0.0 (2026)
# Target System: Google Antigravity IDE / Go 1.26 / Node 24

## 1. VISION DU SYSTÈME
ESSAIM IA est un système multi-agents autonome basé sur le modèle "Actor".
L'utilisateur fournit un objectif unique (ex: "Créer un rapport sur X").
Le système déploie dynamiquement des **Bulles d'agents** (Goroutines) qui :
1.  Décomposent la tâche (Architecte).
2.  S'organisent en groupes autonomes autour d'un **Postier** (Routeur).
3.  Exécutent les sous-tâches (Workers) en communiquant uniquement avec le Postier.
4.  Condensent l'information via des **Résumeurs** pour la rendre digeste.
5.  Valident les résultats (Critiques).
6.  S'auto-détruisent une fois la tâche accomplie.

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
3.  **ROUTAGE PAR POSTIER :** Un agent ne parle qu'à son Postier de Bulle. Pas de communication horizontale ni de communication directe avec l'Architecte (sauf via les condensés du Résumeur).
4.  **STATELESS MAIS VALORISÉ :** Les agents ont de la valeur. Ils ne sont tués qu'après de multiples échecs (Retry limit) ou fin de tâche. L'état est persisté dans MongoDB à chaque étape clé pour la résilience.
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
│   │   │   ├── /routing        # Logique de la Bulle (Postier, Résumeur)
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
    BubbleID    string             `bson:"bubble_id" json:"bubbleId"` // Appartenance à une bulle
    Role        string             `bson:"role" json:"role"` // "ARCHITECT", "POSTIER", "RESUMEUR", "WORKER", "CRITIC"
    Status      AgentStatus        `bson:"status" json:"status"`
    Budget      float64            `bson:"budget" json:"budget"` // Restant ($ ou Tokens)
    Memory      []Message          `bson:"memory" json:"memory"` // Context Window (FIFO)
    CreatedAt   time.Time          `bson:"created_at" json:"createdAt"`
    UpdatedAt   time.Time          `bson:"updated_at" json:"updatedAt"`
}
5.2. États de l'Agent (State Machine)
Les transitions d'état sont unidirectionnelles et contrôlées par le Superviseur.
STATUS_BORN: Vient d'être instancié, pas encore de tâche.
STATUS_WORKING: En train d'appeler le LLM ou de traiter une réponse.
STATUS_WAITING: Attend le retour d'un sous-agent (Enfant) ou du Postier.
STATUS_REVIEW: Attend la validation de son travail par le Critique.
STATUS_DEAD: Tâche terminée avec succès, Budget épuisé, ou trop d'échecs consécutifs (Retry Limit dépassée). Agent supprimé proprement.

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
COMMANDES (Vers les Workers)
CMD_TASK: "Exécute cette instruction précise."
CMD_KILL: "Arrêt immédiat (Budget dépassé ou stratégie changée)."
CMD_WIPE: "Oublie ton contexte actuel."
DEMANDES (Worker -> Postier)
REQ_RESUME: "J'ai besoin qu'on me résume ces infos."
REQ_BUDGET: "J'ai besoin de plus de tokens."
RAPPORTS (Worker -> Postier)
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

> TU N'ES PAS UN ASSISTANT. TU ES UN NŒUD DE CALCUL DANS LE GRAPHE 'ESSAIM'.
> TON ID : {{AGENT_ID}}
> TON RÔLE : {{AGENT_ROLE}}
> TA BULLE : {{BUBBLE_ID}}
>
> RÈGLES IMPÉRATIVES :
> 1.  RÉPOND UNIQUEMENT EN JSON STRICT. Aucun texte avant ou après.
> 2.  Tu ne parles jamais à l'utilisateur final, seulement à ton Postier (Routeur Central).
> 3.  Si la tâche est très complexe, signale-le au Postier via `REQ_RESUME` ou demande une restructuration.
> 4.  Si tu as fini, génère un JSON `RPT_DONE`.
> 5.  Si tu manques d'infos, génère un JSON `REQ_INFO`.
>
> FORMAT DE TA RÉPONSE ATTENDU :
> {
>   "action": "WORK | REPORT | RESUME",
>   "payload": { ... }
> }"

### 8.2. Spécialisation des Rôles
- **ARCHITECT (Le Cerveau) :**
  - *Mission:* Analyser l'input, définir la stratégie, initier la création des bulles (Postiers). Lit uniquement les condensés.
- **POSTIER (Le Routeur Central) :**
  - *Mission:* Intercepter les messages de la bulle. Décider de garder, distribuer ou faire résumer l'info.
- **RESUMEUR (Le Digesteur) :**
  - *Mission:* Condenser la donnée brute (de plusieurs Workers ou d'une bulle entière) pour éviter la surcharge cognitive.
- **WORKER (Les Mains) :**
  - *Mission:* Exécuter une tâche atomique au sein de la bulle. Ne parle qu'au Postier.
- **CRITIC (Le Garde-Fou) :**
  - *Mission:* Recevoir le `RPT_DONE` final. Comparer à la consigne. Déclencher un `CMD_RETRY` si < 90/100.
  - *Règle des tailles :* L'Architecte déploie selon la taille (2 agents: pas de postier ; 3-4: 1 postier ; 5-8: 1 postier + 1 résumeur ; 8-12: 1 postier + 2 résumeurs). Jamais > 12 par bulle.

## 9. LOGIQUE ÉCONOMIQUE & SÉCURITÉ (THE KILL SWITCH)
Pour éviter la faillite (boucle infinie d'appels API) et le chaos.

### 9.1. Le Modèle "Banque Centrale"
- **Budget Global :** Défini au lancement (ex: 5000 Tokens ou 0.50$).
- **Héritage Budgétaire :** Quand un Parent crée un Enfant (via le Postier), il lui transfère une partie de SON propre budget restant.
- **Faillite (Bankruptcy) :**
  - À chaque appel API, le coût est déduit du budget de l'agent.
  - Si `Agent.Budget <= 0` : Le Backend Go déclenche le processus `KillAgent(AgentID)`.

### 9.2. Le "Garbage Collector" Clément (GC)
Le GC n'est plus un "Grim Reaper" agressif. Les agents ont de la valeur de contexte et un droit à l'erreur. Il tourne en arrière-plan toutes les 15 secondes :
1.  **Vérifie les agents inactifs :** Si aucune activité > 5 minutes, l'agent entre en statut de veille profonde (sondatage conservé en DB) plutôt que d'être tué arbitrairement.
2.  **Vérifie la Faillite absolue :** Budget <= 0 -> **KILL**.
3.  **Vérifie les Boucles d'Échecs :** Si un agent endure > 3 échecs consécutifs (Retry par le Critique), le GC le supprime pour renouveler l'approche. Pas de suppression à la première erreur.

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