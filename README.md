# api-gallery

Galerie photo authentifiée, sans base de données.
Stack : **Go** + système de fichiers + **auth-service** (magic link) + **VPS Ionos** via Docker + Traefik

---

## Structure

```
api-gallery/
├── cmd/server/main.go              # point d'entrée
├── internal/
│   ├── config/config.go            # variables d'environnement
│   ├── index/index.go              # scan du système de fichiers (albums/photos), en mémoire
│   ├── thumbnail/thumbnail.go      # génération + cache disque des vignettes
│   ├── authmw/middleware.go        # middleware Bearer, valide via auth-service /auth/me
│   └── httpapi/handlers.go         # routes HTTP
├── .github/workflows/docker.yml   # CI/CD GitHub Actions
├── Dockerfile
└── .env.example
```

Pas de base de données : la liste des albums/photos est reconstruite en mémoire à chaque
démarrage et à chaque appel à `POST /admin/refresh`, en parcourant `PHOTOS_ROOT`.

Convention de contenu : chaque sous-dossier de premier niveau sous `PHOTOS_ROOT` est un
album ; les fichiers `.jpg`/`.jpeg`/`.png` directement dans un dossier album sont ses photos
(sous-dossiers imbriqués, vidéos et autres formats ignorés en V1).

---

## 1. Prérequis

- Go 1.22+
- Docker + Docker Compose sur le VPS
- Une instance d'[auth-service](https://github.com/polpoul/auth-service) déjà déployée
- Réseau Docker `web` (déjà créé pour auth-service)
- Un dossier de photos déjà synchronisé sur le VPS (rsync/scp manuel), organisé en
  `PHOTOS_ROOT/<album>/<photo>.jpg`

---

## 2. Développement local

```bash
cp .env.example .env
# remplir PHOTOS_ROOT / AUTH_SERVICE_URL / CORS_ORIGINS

go mod tidy
go run ./cmd/server
```

Test rapide (nécessite un `device_token` valide obtenu via auth-service) :

```bash
curl http://localhost:8080/health
# → ok

curl http://localhost:8080/albums \
  -H "Authorization: Bearer DEVICE_TOKEN"

curl http://localhost:8080/albums/mon-album/photos \
  -H "Authorization: Bearer DEVICE_TOKEN"

curl -X POST http://localhost:8080/admin/refresh \
  -H "Authorization: Bearer DEVICE_TOKEN"
```

---

## 3. Déploiement sur le VPS

### Étape 1 — Pusher sur GitHub

```bash
git add . && git commit -m "update"
git push
```

GitHub Actions build et publie automatiquement l'image sur `ghcr.io/polpoul/api-gallery:latest`.
Watchtower met à jour le container sur le VPS toutes les 5 minutes.

### Étape 2 — Ajouter le service au compose existant

Dans `~/traefik/docker-compose.yml` :

```yaml
api-gallery:
  image: ghcr.io/polpoul/api-gallery:latest
  restart: unless-stopped
  networks:
    - web
  environment:
    - PHOTOS_ROOT=/data/photos
    - CACHE_DIR=/data/cache
    - AUTH_SERVICE_URL=https://auth.vivalink.top
    - CORS_ORIGINS=https://gallery.vivalink.top
    - PORT=8080
  volumes:
    - /srv/vivalink/photos:/data/photos:ro
    - gallery-cache:/data/cache
  labels:
    - traefik.enable=true
    - traefik.http.routers.api-gallery.rule=Host(`api-gallery.vivalink.top`)
    - traefik.http.routers.api-gallery.entrypoints=websecure
    - traefik.http.routers.api-gallery.tls.certresolver=<identique à auth-service>
    - traefik.http.services.api-gallery.loadbalancer.server.port=8080

volumes:
  gallery-cache:
```

### Étape 3 — Synchroniser les photos

```bash
rsync -avz --delete /chemin/local/albums/ pascal@<vps>:/srv/vivalink/photos/
```

Puis rafraîchir l'index (ou attendre le prochain redémarrage du service) :

```bash
curl -X POST https://api-gallery.vivalink.top/admin/refresh \
  -H "Authorization: Bearer DEVICE_TOKEN"
```

### Étape 4 — Lancer les services

```bash
cd ~/traefik
docker-compose up -d
```

### Étape 5 — Vérifier

```bash
curl -k https://localhost/health -H "Host: api-gallery.vivalink.top"
# → ok

curl https://api-gallery.vivalink.top/health
# → ok
```

### Étape 6 — DNS sur Ionos

Ajouter un enregistrement :
- **Type** : `A`
- **Nom** : `api-gallery`
- **Valeur** : `87.106.43.140`

---

## 4. Autoriser l'app sur auth-service

`api-gallery` ne fait que valider les tokens via `auth-service` — aucune modification de
code n'y est nécessaire, seulement de la configuration sur le déploiement existant :

- Ajouter `https://gallery.vivalink.top` à la variable d'env `CORS_ORIGINS` d'auth-service
  (pour que le frontend de la galerie puisse appeler `/auth/request-login` et `/auth/logout`
  directement depuis le navigateur).
- Si une allowlist par email/app est configurée (`ALLOWLIST_PATH`), ajouter `"gallery"` à la
  liste des apps autorisées pour les emails concernés.

---

## 5. Intégration frontend (voir `apps-vivalink/apps/gallery`)

Même flow que les autres apps vivalink (`agenda`, `voyage`...) : pas de session serveur,
juste un `device_token` en `localStorage`, et un aller-retour sur la page d'accueil de
l'app elle-même (pas de page de callback séparée) :

1. `POST https://auth.vivalink.top/auth/request-login` avec
   `{"email": "...", "app": "gallery", "redirect_url": "https://gallery.vivalink.top"}`.
2. L'utilisateur clique le lien reçu par email → revient sur
   `gallery.vivalink.top?token=XXX`.
3. Au chargement, `index.html` lit `?token=` dans l'URL, appelle
   `GET https://auth.vivalink.top/auth/login?token=XXX`, récupère
   `{device_token, user_id}`, le stocke en `localStorage`, puis nettoie l'URL.
4. Chaque appel à l'API gallery envoie `Authorization: Bearer <device_token>`.

---

## API

| Méthode | Route | Auth | Description |
|---------|-------|------|-------------|
| GET | `/health` | — | Health check |
| GET | `/albums` | Bearer | Liste des albums |
| GET | `/albums/:albumId/photos` | Bearer | Liste des photos d'un album |
| GET | `/albums/:albumId/photos/:photoId` | Bearer | Image originale |
| GET | `/albums/:albumId/photos/:photoId/thumb` | Bearer | Vignette (générée puis cachée sur disque) |
| POST | `/admin/refresh` | Bearer | Force le re-scan du système de fichiers |

---

## Variables d'environnement

| Variable | Description |
|----------|-------------|
| `PHOTOS_ROOT` | Racine des albums (montée en lecture seule) |
| `CACHE_DIR` | Dossier de cache des vignettes générées (volume persistant) |
| `AUTH_SERVICE_URL` | URL d'auth-service, ex : `https://auth.vivalink.top` |
| `CORS_ORIGINS` | Origines autorisées en cross-origin (frontend gallery), séparées par des virgules |
| `PORT` | Port d'écoute (défaut : `8080`) |
