# PHILOSOPHIE DU SYSTÈME : ESSAIM-IA (HIVE)

Ce document centralise l'essence, les principes théoriques et les règles immuables d'ESSAIM-IA. Il a été rédigé à votre demande ("j'ai beaucoup parlé pour que ce soit utile pour la prochaine fois...") pour condenser et retenir de manière persistante toute votre vision fondatrice du projet. Il sert de boussole architecturale et intellectuelle, garantissant que toute évolution future (y compris par une IA) respectera l'âme du système tel que vous l'avez pensé.

## 1. L'ESSENCE DU PROJET : PURETÉ ET AUTONOMIE

ESSAIM-IA n'est pas un banal assistant conversationnel. C'est un *véritable système d'intelligence artificielle distribuée*, basé sur le modèle **Actor**.
L'humain (l'utilisateur) n'est qu'un initiateur : il donne un objectif macroscopique ("générer un rapport", "coder une app"), l'Essaim se charge du reste. Une fois la graine plantée, l'humain devient un simple spectateur dans son "Cockpit".
L'interaction humaine est strictement limitée à l'observation (Monitoring) et à l'arrêt d'urgence (Kill Switch).

## 2. LES 5 LOIS FONDAMENTALES (RÈGLES D'OR)

Toute ligne de code écrite dans ce projet ou toute nouvelle consigne doit se plier à ces lois d'airain :

1. **Silence Radio Absolu :** Les agents ne sont pas là pour faire la conversation. Ils communiquent exclusivement via des structures de données (JSON strict). Tout texte libre est traité comme un poison et une erreur mortelle pour le parseur.
2. **Organisation en Bulles (Postiers & Résumeurs) :** Fini la hiérarchie strictement verticale. Les agents exécutants s'organisent en "Bulles circulaires" autour d'un **Postier**. Un agent ne parle qu'à son Postier. Le Postier route, et les **Résumeurs** condensent l'information pour éviter de surcharger le contexte de l'Architecte ou des autres agents. L'Architecte ne lit que des condensés. Pas de chat horizontal direct inter-agents (sauf via mémoire partagée persistée).
3. **Logique "Stateless" mais Valorisée (Droit à l'Erreur) :** Contrairement aux anciens systèmes, un agent a de la valeur. Il n'est pas qu'un ouvrier "jetable" à la moindre erreur. Son état est persisté dans MongoDB pour la résilience. Un agent n'est supprimé qu'après de multiples échecs consécutifs (Retry Cycles épuisés) ou la complétion totale de sa tâche. Le Garbage Collection est clément et n'intervient qu'en dernier recours.
4. **Non-Blocking I/O (L'Orchestration Asynchrone Maximale) :** Les appels API externes (Gemini/Claude) ne doivent bloquer aucun flux vital du système. Le backend Go base son efficience sur un Worker Pool et un Dispatcher, assurant que 5 000 agents logiques puissent vivre sur seulement 50 threads API en simultané.

## 3. LA MÉTAPHORE BIOLOGIQUE ET ÉCONOMIQUE

- **La Mitose (Spawn) :** Lorsqu'une tâche donnée à l'Essaim est complexe, l'Architecte décide du nombre d'agents nécessaires. Le système déploie automatiquement l'infrastructure (Postiers/Résumeurs) adaptée. Le modèle idéal est des groupes de 3 à 5 exécutants. (Règle : 2 agents = pas de Postier ; 3-4 agents = 1 Postier ; 5-8 = 1 Postier + 1 Résumeur ; 8-12 = 1 Postier + 2 Résumeurs). La limite matérielle d'une bulle est de 12 agents.
- **L'Enveloppe (La Communication IAP) :** Le protocole IAP (Inter-Agent Protocol) est l'ADN standardisé des échanges. Une requête contient systématiquement l'origine, la destination (souvent le Postier de la bulle), l'action très stricte et atomique. Ainsi l'IA "ne délire pas sur le conteneur", elle se concentre uniquement sur la Payload intelligente.

## 4. LA SPHÈRE DES RÔLES ET LES INJECTIONS DE PERSONNALITÉS

Chaque agent est instancié avec un rôle bien défini, et reçoit le "Master Prompt" qui lui rappelle sévèrement qu'il N'EST PAS UN ASSISTANT.

- **L'Architecte (Le Cerveau Planificateur) :** Biaisé pour une Haute Logique, et Faible Créativité. Il conçoit, décompose la macro-tâche, identifie les besoins, et crée les groupes de Workers. Il ne lit jamais l'information brute, uniquement les condensés de ses Résumeurs de sortie.
- **Le Postier (Le Routeur) :** Le cœur de la bulle. Il intercepte les productions des Workers. Il décide intelligemment s'il garde l'information ou s'il la redistribue (éventuellement via un Résumeur) à un autre agent du groupe pour l'aider. Les Workers ne parlent qu'à lui.
- **Le Résumeur (La Clé de la Scalabilité) :** Agent spécialisé dans la digestion de la donnée. Il aide le Postier à condenser les informations pour le groupe, ou compile le travail de toute la bulle (Résumeur de sortie) pour l'Architecte.
- **Le Worker (Les Mains Exécutantes) :** Biaisé pour une Grande Précision et un Respect Rigide de ce qui lui est ordonné. Il accomplit une tâche atomique au sein de sa bulle.
- **Le Critique (Le Garde-Fou Interne) :** Rôle asynchrone qui évalue objectivement le produit fini d'un Worker ou du groupe. Si le score est insuffisant, il ordonne un "Retry Cycle". L'agent fautif n'est pas tué immédiatement : il a le droit d'apprendre de ses erreurs (jusqu'à une limite définie) avant d'être écarté.

## 5. REPRÉSENTATION ET OBSERVABILITÉ (LE COCKPIT FRONTEND)

- L'interface n'héberge aucune logique métier du système de l'Essaim : elle a uniquement un rôle de tour de contrôle et de **transparence absolue de ce qui s'y passe.**
- Elle affiche l'état temps réel pour être le tableau de bord esthétique (Nœuds, Flux de Particules).
- Le **God Mode** permet l'inspection approfondie d'un Agent (le voir réfléchir, voir son prompt, son id, son budget et ses flux).
- Et un unique bouton fatidique : **TERMINATE BRANCH**, un Kill Switch récursif, manuel et d'urgence si l'Observateur décide d'avorter une branche ou le projet entier.

## 6. POURQUOI AVOIR RÉDIGÉ CELA ?

Afin d'assurer la pérennité du projet, des futures sessions de travail et de codage.
Tous les outils à venir, tous les agents (comme moi) qui viendront coder ce système dans le futur s'imprégneront de ces règles, ancrées dans `PHILOSOPHIE.md` et dans `ARCHITECTURE.spec`.
Ces mots sont le ciment qui fait d'ESSAIM-IA non pas un script éphémère, mais un projet "Massivement Parallélisable, Résilient, Contrôlable Financièrement, et Sans Compromis".
