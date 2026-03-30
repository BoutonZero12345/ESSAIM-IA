# Roadmap Stratégique & Liste de Tâches — ESSAIM-IA

Ce document consolide la vision à long terme du projet et les étapes de réalisation (Task List) pour les prochaines semaines.

## Axe 1 : 🛠️ De la Théorie à l'Action (Intégration d'Outils - "Tool Callings")

*Actuellement nos agents "réfléchissent" et écrivent beaucoup, mais ils sont bloqués dans une boîte textuelle. Il faut leur donner des bras virtuels.*

- [ ] **Fondations "Outils" :** Créer un package `internal/domain/tools/` pour standardiser les interfaces (Nom, Description, Fonction Execute()).
- [ ] **Outil I/O Fichier :** Implémenter un outil permettant à un agent de créer ou modifier dynamiquement un fichier (ex: `WriteFileWorker`).
- [ ] **Outil Accès Web :** Implémenter un outil permettant de lancer une requête HTTP simple ou via DuckDuckGo pour ramener du contexte externe.
- [ ] **Exécution Code Sandbox :** (Plus Complexe) Créer un environnement sécurisé pour lancer des scripts Python ou Go générés par l'IA.

## Axe 2 : 🧠 Scalabilité Cognitive (Mémoire Globale & Intelligence)

*Même si la synthèse locale (Résumeur) fonctionne, le système manque de recul globalisant d'une mission à l'autre.*

- [ ] **Mémoire Vectorielle (RAG) :** Stocker le payload final des bilans et des `RPT_DONE` majeurs en base vectorielle avec tag sémantique. L'Agent `Architecte` (Alpha) pourra s'y référer pour se souvenir des erreurs passées avant d'allouer de l'argent et des ressources.
- [ ] **Heuristique Anti-Boucle :** Implémenter dans le `Processor` un compteur global qui détecte et tue une bulle si elle s'échange le même texte X fois dans le vide.
- [ ] **Spécialisation des Prompts :** Déporter la génération de prompts (actuellement unique dans `agent/prompt.go`) vers des templates hautement spécialisés par rôle (ex: Chercheur vs Testeur vs Rédacteur).

## Axe 3 : 🌐 La Renaissance du Frontend (Interface d'Observation "God Mode")

*Nous avons jeté l'ancien frontend car il était illisible. Nous le recréerons quand le backend sera parfait, mais de façon magnifique et ambitieuse.*

- [ ] **Scaffolding Frontend :** Initialiser un projet Next.js / React propre sans dette technique.
- [ ] **Visualisation Graphe 3D/Nodale :** Connecter la WebSocket et afficher la création/destruction des noeuds (agents) et des bulles en direct (via React Flow ou Three.js).
- [ ] **Dashboard Économie :** Graphique macro en temps-réel de l'évaporation des tokens de budget et des fallbacks budgétaires.
- [ ] **Interruption Humaine (Live) :** Permettre l'envoi d'un signal "Pause" ou d'une "Nouvelle Instruction" à une bulle spécifique depuis l'interface, sans relancer toute la mission.

---

*Document généré le 27 Mars 2026. L'outil servira de boussole pour le développement futur.*
