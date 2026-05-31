# 2026-03-27 — Découpage et Lisibilité du Processor

## Ce que j'ai dit
Suite à la demande de l'utilisateur, j'ai découpé l'immense fichier `processor.go` (1100+ lignes) en 6 fichiers plus courts et ultra-spécialisés. Le but était d'augmenter la lisibilité et de structurer l'architecture sans altérer une seule ligne de logique métier existante.

## Ce que j'ai fait

### Fichiers créés / fractionnés
- `backend/internal/core/lifecycle/types.go` : Création pour centraliser les structures JSON (`LLMAction`, `SpawnPayload`, etc).
- `backend/internal/core/lifecycle/utils.go` : Extraction des helpers (`killAgent`, `reportToParent`, `broadcast*`).
- `backend/internal/core/lifecycle/spawn.go` : Extraction stricte de la logique de délégation (`handleSpawn`, parsesLLM).
- `backend/internal/core/lifecycle/work.go` : Extraction de la logique d'exécution directe (`handleWork`).
- `backend/internal/core/lifecycle/report.go` : Extraction de la remontée de statut et des résumeurs (`handleReport`, `handlePostierReport`).
- `backend/internal/core/lifecycle/processor.go` : Allégé à ~280 lignes. Il ne contient plus que l'ossature `Processor`, `NewProcessor`, et le routeur primaire `HandlePacket` / `handleTask`.

## Résumé des modifications
- `types.go`, `utils.go`, `spawn.go`, `work.go`, `report.go` : Créés pour scinder les responsabilités du `processor.go` par domaine logique.
- `processor.go` : Suppressions massives des méthodes déléguées. Garde uniquement la définition et le coeur de l'orchestrateur. Le test a confirmé que 100% du code reste fonctionnel (21/21 builds et tests OK).
