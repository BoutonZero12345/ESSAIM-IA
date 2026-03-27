# Démarrage Rapide (ESSAIM-IA)

Voici les commandes essentielles pour lancer l'application ESSAIM-IA dans votre terminal.

### 1. Lancer le Backend (Serveur Go)

Ouvrez un terminal, placez-vous dans le dossier `backend` et démarrez le serveur :

```bash
cd backend
go run cmd/server/main.go
```

*(Le serveur écoutera sur le port 8080)*

### 2. Lancer le Frontend (Interface React)

Ouvrez un **deuxième terminal**, placez-vous dans le dossier `reseau-graph` (ou le dossier contenant votre UI) et démarrez-le :

```bash
cd frontend
npm run dev
# ou si compilé : npx serve -s build
```

Vous pouvez ensuite ouvrir l'application dans votre navigateur pour voir l'essaim se déployer !
