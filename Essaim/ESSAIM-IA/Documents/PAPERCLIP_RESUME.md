# RECHERCHES SUR PAPERCLIP AI

Suite à votre demande de recherche sur le projet GitHub "Paperclip AI", voici un résumé des découvertes. Il existe en réalité deux projets récents portant ce nom, mais l'un d'eux résonne tout particulièrement avec la vision d'ESSAIM-IA.

## 1. LE PROJET PRINCIPAL : paperclipai/paperclip
**"Open-source orchestration for zero-human companies"**

Ce projet (disponible sur [GitHub - paperclipai/paperclip](https://github.com/paperclipai/paperclip)) est une plateforme d'orchestration open-source dont le but est de créer des "entreprises sans humains".

### En quoi ça consiste ?
- C'est un orchestrateur agnostique : il ne fournit pas les modèles d'IA, mais il se charge de les manager. Vous apportez vos agents (Claude Code, OpenClaw, Codex, Cursor), et Paperclip les coordonne.
- Il permet de définir des **buts (goals)**, de créer des **organigrammes (org charts)**, et surtout de **gérer des budgets**.

### Pourquoi c'est très intéressant pour ESSAIM-IA ?
Paperclip AI aborde exactement les mêmes problématiques que celles que vous avez théorisées pour ESSAIM-IA :
1. **La gestion de budget :** Paperclip permet d'imposer des limites de budget aux agents, tout comme la règle de "Banqueroute" d'ESSAIM-IA.
2. **Le monitoring :** Il fournit un dashboard complet pour voir ce que font les agents, combien ils coûtent, et permet à l'humain d'auditer et d'intervenir uniquement si nécessaire (ce qui rappelle le "Cockpit" d'ESSAIM-IA).
3. **L'orchestration asynchrone :** Les agents tournent en continu 24/7 sur la base de "heartbeats" (battements de cœur) ou d'événements déclencheurs (assignation de tâche).
4. **La notion de "Entreprise/Groupe" :** Le fait que les agents s'organisent en entreprise fait écho à votre vision récente des **Bulles de travail (Postiers/Résumeurs)** plutôt qu'à une simple hiérarchie verticale.

### Comment ça marche techniquement ?
- Un serveur Node.js (v20+) avec une base de données PostgreSQL embarquée.
- Tout est gérable via des commandes (ex: `npx paperclipai onboard --yes`).
- Il est conçu pour être auto-hébergé.

---

## 2. L'AUTRE PROJET : fredruss/agent-paperclip
**"Desktop 'Paperclip' Companion for Claude Code & Codex"**

(Lien : [GitHub - fredruss/agent-paperclip](https://github.com/fredruss/agent-paperclip))
Ce projet-ci est plus anecdotique mais amusant. C'est un petit widget de bureau (façon le vieux trombone de Microsoft, *Clippy*) conçu avec Electron.
- Il se connecte aux logs de *Claude Code* ou *Codex*.
- Il affiche sur votre écran ce que fait l'agent en arrière-plan (ex: "Je lis index.ts...", "Je cherche un pattern...", ou "Je réfléchis...").
- C'est purement visuel (monitoring local), ça ne gère pas l'orchestration globale.

---

## CONCLUSION ET INSPIRATION POUR ESSAIM-IA
Le projet **paperclipai/paperclip** confirme que l'approche d'ESSAIM-IA (budget fini, agents de différents rôles, orchestration centralisée, supervision asynchrone) est totalement dans l'air du temps et correspond à la direction que prend l'intelligence artificielle agentique aujourd'hui. Il serait probablement intéressant de regarder comment *Paperclip AI* gère techniquement son "heartbeat" et ses budgets dans le code source pour s'en inspirer dans l'implémentation Go d'ESSAIM-IA.
