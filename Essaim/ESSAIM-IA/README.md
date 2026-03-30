![ESSAIM-IA Banner](assets/banner.png)

<div align="center">
  <h1>🐝 ESSAIM-IA</h1>
  <p><b>Orchestrateur Fractal d'Agents Autonomes en Go</b></p>
  <p><i>L'intelligence collective par l'isolation et la délégation hiérarchique.</i></p>
  <p>🚀 <b>Totalement vibe codé avec l'outil Antigravity</b></p>

  <img src="https://img.shields.io/badge/Language-Go-00ADD8?style=for-the-badge&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/LLM-Gemini_2.5-4285F4?style=for-the-badge&logo=google-gemini" alt="Gemini">
  <img src="https://img.shields.io/badge/Architecture-Fractal_Bubbles-FFD700?style=for-the-badge" alt="Bubbles">
  <img src="https://img.shields.io/badge/Status-Beta_Technical_Preview-green?style=for-the-badge" alt="Status">
</div>

---

## 🌌 Vision
Les systèmes d'IA multi-agents classiques souffrent souvent du "bruit contextuel" : trop d'agents se parlent en même temps, saturant les fenêtres de contexte et provoquant des hallucinations collectives.

**ESSAIM-IA** résout ce problème par une architecture de **Délegation Fractale**. Inspiré par les structures organisationnelles militaires et biologiques, le projet segmente le travail en "Bulles" (Closed Groups) isolées, où l'information ne remonte que sous forme de synthèse consolidée.

## 🏗️ Architecture Système (The "Bubble" Pattern)

### 1. Les Bulles de Contexte (Closed Groups)
Chaque tâche complexe déclenche un `SPAWN`. Les agents créés sont enfermés dans une **Bulle**. Ils ne peuvent pas communiquer avec l'extérieur, seulement entre eux et avec leur superviseur. Cela garantit une focalisation maximale sur l'objectif local.

### 2. Le Postier & Le Résumeur
Pour scalabiliser la hiérarchie sans perdre en clarté, ESSAIM-IA implante des agents middleware automatiques :
- **Le Postier** : Gère le routage des messages dans les groupes de plus de 2 agents (Évite le couplage fort).
- **Le Résumeur** : Invoqué automatiquement lorsque le volume de données dépasse les seuils critiques (5+ agents), il synthétise les travaux avant de les transmettre au parent.

### 3. Cycle de Vie Séquentiel (Sequential Multi-Turn)
Contrairement aux systèmes "One-Shot", ESSAIM-IA permet aux agents de "se réveiller". Un agent parent qui délègue une tâche ne meurt pas ; il entre en sommeil, puis est réactivé par l'orchestrateur Go une fois les résultats reçus pour analyser, valider ou relancer une nouvelle phase de travail.

## 🛠️ Stack Technique
- **Backend Core** : Go (Golang) — Choisi pour sa gestion native de la concurrence (Channels & Goroutines) et sa robustesse système.
- **Moteur d'Intelligence** : Google Gemini 2.5 Flash — Optimisé pour la vitesse et les larges fenêtres de contexte.
- **Persistance** : MongoDB — Stockage des états d'agents et des journaux de mission.
- **Real-time** : WebSockets — Orchestration et monitoring en temps réel.

## 🚀 Démarrage Rapide

### Pré-requis
- Go 1.21+
- Clé API Gemini
- MongoDB local ou Atlas

### Installation
```bash
# Cloner le dépôt
git clone https://github.com/votre-compte/essaim-ia
cd essaim-ia/backend

# Configurer l'environnement
cp .env.example .env # Ajoutez vos clés
go mod download

# Lancer le serveur
go run cmd/server/main.go
```

## 📊 Monitoring & Évaluation
Chaque mission d'ESSAIM-IA génère un **Bilan de Mission** (`BILAN_<timestamp>.md`). Ce document détaille :
- Le taux de réussite des workers.
- L'efficacité budgétaire (Tokens utilisés vs alloués).
- La structure de la hiérarchie créée dynamiquement.

## 🗺️ Roadmap Stratégique
- [ ] **Axe I : Tool Calling** intégration de l'I/O système (lecture/écriture de fichiers).
- [ ] **Axe II : Long Term Memory** implémentation d'une base de données vectorielle (RAG).
- [ ] **Axe III : God Mode UI** reconstruction d'un frontend de visualisation graphique nodal.

---

<p align="center"><i>Développé avec passion pour repousser les limites de l'autonomie agentique.</i></p>
