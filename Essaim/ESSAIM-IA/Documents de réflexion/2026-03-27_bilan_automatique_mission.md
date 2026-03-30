# 2026-03-27 — Système de Bilan Automatique de Mission

## Ce que j'ai dit
Ajout d'un système de bilan automatique généré à chaque fin de mission, sous forme d'un fichier Markdown de moins d'une page. Ce bilan est produit sans appels LLM supplémentaires — il s'appuie entièrement sur les métriques collectées pendant l'exécution.

## Ce que j'ai fait

### Fichiers créés
- `backend/internal/core/evaluation/evaluator.go` : nouveau package `evaluation` — struct `MissionStats` (compteurs thread-safe), `GenerateBilan()` (génération markdown heuristique), `WriteBilan()` (écriture fichier)
- `backend/internal/core/evaluation/evaluator_test.go` : 6 tests unitaires (mission parfaite, fallbacks, confiance faible, format < 1 page, 0 retries, résumeur mentionné)

### Fichiers modifiés
- `backend/internal/core/lifecycle/processor.go` : ajout du champ `*evaluation.MissionStats` sur `Processor`, méthode `SetObjective()`, hooks métriques dans `handleSpawn` (RecordSpawn, RecordPostier), `callResumeur` (RecordResumeur), `handleWork` (RecordWorkerDone + SetFinalConfidence), `killAgent` (RecordWorkerFailed), `handleSpawn` fallback (RecordFallback), `reportToParent` root path (Finish + WriteBilan)
- `backend/cmd/server/main.go` : appel `processor.SetObjective()` dans le handler `/start`

## Résumé des modifications
- `evaluator.go` : nouveau package autonome, pas d'import LLM, logique purement heuristique sur les métriques collectées
- `evaluator_test.go` : 6 tests validant les cas de succès, d'échec, de confiance faible, et la contrainte de taille (< 65 lignes)
- `processor.go` : métriques incrémentées aux 6 points clés du cycle de vie des agents, bilan écrit dans `backend/logs/bilans/BILAN_<timestamp>.md` à la fin de chaque mission
- `main.go` : SetObjective() appelé pour initialiser le bilan avec le bon objectif de mission
