# PHILOSOPHIE DU SYSTÈME : ESSAIM-IA (HIVE)

Ce document centralise l'essence, les principes théoriques et les règles immuables d'ESSAIM-IA. Il a été rédigé à votre demande ("j'ai beaucoup parlé pour que ce soit utile pour la prochaine fois...") pour condenser et retenir de manière persistante toute votre vision fondatrice du projet. Il sert de boussole architecturale et intellectuelle, garantissant que toute évolution future (y compris par une IA) respectera l'âme du système tel que vous l'avez pensé.

## 1. L'ESSENCE DU PROJET : PURETÉ ET AUTONOMIE

ESSAIM-IA n'est pas un banal assistant conversationnel. C'est un *véritable système d'intelligence artificielle distribuée*, basé sur le modèle **Actor**.
L'humain (l'utilisateur) n'est qu'un initiateur : il donne un objectif macroscopique ("générer un rapport", "coder une app"), l'Essaim se charge du reste. Une fois la graine plantée, l'humain devient un simple spectateur dans son "Cockpit".
L'interaction humaine est strictement limitée à l'observation (Monitoring) et à l'arrêt d'urgence (Kill Switch).

## 2. LES 5 LOIS FONDAMENTALES (RÈGLES D'OR)

Toute ligne de code écrite dans ce projet ou toute nouvelle consigne doit se plier à ces lois d'airain :

1. **Silence Radio Absolu :** Les agents ne sont pas là pour faire la conversation. Ils communiquent exclusivement via des structures de données (JSON strict). Tout texte libre est traité comme un poison et une erreur mortelle pour le parseur.
2. **Hiérarchie Stricte (Le Graphe Orienté) :** La communication a lieu de façon verticale. Un nœud (Agent) ne parle qu'à son créateur (Parent) pour faire son rapport (`REPORT`), ou à ses sous-agents (Enfants) pour requérir une action ou donner des ordres (`COMMAND`). Pas de chat horizontal direct inter-agents (sauf via mémoire partagée persistée).
3. **Logique "Stateless" (La Réplicabilité Temporelle) :** Un agent en cours d'exécution (Goroutine) n'est rien d'autre qu'un ouvrier jetable et peut être tué arbitrairement. Toute mémoire de travail critique et transition d'état doit être persistée dans MongoDB pour permettre une reprise sur incident, ou un Garbage Collection pur.
4. **Non-Blocking I/O (L'Orchestration Asynchrone Maximale) :** Les appels API externes (Gemini/Claude) ne doivent bloquer aucun flux vital du système. Le backend Go base son efficience sur un Worker Pool et un Dispatcher, assurant que 5 000 agents logiques puissent vivre sur seulement 50 threads API en simultané.

## 3. LA MÉTAPHORE BIOLOGIQUE ET ÉCONOMIQUE

- **La Mitose (Spawn) :** Lorsqu'une tâche donnée à un agent est trop complexe (généralement estimée à plus de 3 sous-étapes), sa directive prioritaire est de demander à se diviser. Il délègue ("hante") conditionnellement une partie de son Propre budget pour faire naître un (ou plusieurs) sous-enfant(s) spécialisé(s). Le modèle fractal émerge d'ici.
- **L'Enveloppe (La Communication IAP) :** Le protocole IAP (Inter-Agent Protocol) est l'ADN standardisé des échanges. Une requête contient systématiquement l'origine, la destination, l'action très stricte et atomique (ex: `CMD_SPAWN`, `RPT_DONE`, etc). Ainsi l'IA "ne délire pas sur le conteneur", elle se concentre uniquement sur la Payload intelligente.

## 4. LA SPHÈRE DES RÔLES ET LES INJECTIONS DE PERSONNALITÉS

Chaque agent est instancié avec un rôle bien défini, et reçoit le "Master Prompt" qui lui rappelle sévèrement qu'il N'EST PAS UN ASSISTANT.

- **L'Architecte (Le Cerveau Planificateur) :** Biaisé pour une Haute Logique, et Faible Créativité. Il conçoit, décompose la macro-tâche, identifie les besoins, et crée les Managers.
- **Le Worker (Les Mains Exécutantes) :** Biaisé pour une Grande Précision et un Respect Rigide de ce qui lui est ordonné. Il accomplit une tâche atomique (écrire un module précis de code, scraper un site, extraire quelques données).
- **Le Critique (Le Garde-Fou Interne) :** Rôle asynchrone qui évalue objectivement le produit fini d'un Worker en confrontant le résultat final à la consigne d'origine de l'Architecte. Si le score automatique de validation est insuffisant (ex: <90/100), il ordonne un "Retry Cycle" avec ses arguments et les erreurs qu'il a répertoriées.

## 5. REPRÉSENTATION ET OBSERVABILITÉ (LE COCKPIT FRONTEND)

- L'interface n'héberge aucune logique métier du système de l'Essaim : elle a uniquement un rôle de tour de contrôle et de **transparence absolue de ce qui s'y passe.**
- Elle affiche l'état temps réel pour être le tableau de bord esthétique (Nœuds, Flux de Particules).
- Le **God Mode** permet l'inspection approfondie d'un Agent (le voir réfléchir, voir son prompt, son id, son budget et ses flux).
- Et un unique bouton fatidique : **TERMINATE BRANCH**, un Kill Switch récursif, manuel et d'urgence si l'Observateur décide d'avorter une branche ou le projet entier.

## 6. POURQUOI AVOIR RÉDIGÉ CELA ?

Afin d'assurer la pérennité du projet, des futures sessions de travail et de codage.
Tous les outils à venir, tous les agents (comme moi) qui viendront coder ce système dans le futur s'imprégneront de ces règles, ancrées dans `PHILOSOPHIE.md` et dans `ARCHITECTURE.spec`.
Ces mots sont le ciment qui fait d'ESSAIM-IA non pas un script éphémère, mais un projet "Massivement Parallélisable, Résilient, Contrôlable Financièrement, et Sans Compromis".
