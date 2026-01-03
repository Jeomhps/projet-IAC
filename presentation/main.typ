#import "@preview/silky-slides-insa:0.1.1": *
#import "@preview/oxdraw:0.1.0": *

#show: insa-slides.with(
  title: "Outils de déploiement de plateforme",
  title-visual: none,
  subtitle: "Ghalib Assad Carre Jules Migaud Maxime Stoll Nicolas Amadou Yaya-Diallo",
  insa: "cvl"
)

= Contexte

== Mise en situation

- #text(fill: insa-colors.primary)[Iouri] est un fournisseur de services dans le Darkweb et possède des machines corrompues.
- #text(fill:insa-colors.secondary)[Dimitri] est un cybercriminel et cherche à avoir accès rapidement à une infrastructure pour opérer des attaques cyber en Europe.
  - #text(fill:insa-colors.secondary)[Dimitri] contacte #text(fill:insa-colors.primary)[Iouri] et lui achète des accès à son infrastructure.

== Objectifs scolaires
- Mettre en place une infrastructure de déploiement en utilisant les principes #text(fill:insa-colors.primary)[Infrastructure as a Code] avec un attrait pour un déploiement en #text(fill:insa-colors.secondary)[CI/CD].
- Créer un outil #text(fill:insa-colors.primary)[d'orchestration] d'infrastructure informatique afin de gérer plusieurs ordinateurs à distance.
  - Mettre en place un #text(fill:insa-colors.secondary)[système d'affectation par utilisateurs] (/prestataire) avec un point d'accès #text(fill:insa-colors.tertiary)[centralisé et sécurisé.] 

= Architecture

== Architecture générale

#oxdraw(```mermaid
graph LR
  Client["Client (curl/CLI)"]
  subgraph LAB["lab network"]
    RP["Reverse Proxy (Caddy)<br/>HTTPS :443<br/>Proxies -> API"]
    API["Go API (Gin)<br/>:8080"]
    SCHED["Go Scheduler"]
    DB[(MariaDB)]
  end

  Client -->|HTTPS 443| RP
  RP -->|HTTP :8080| API
  API -->|SQL| DB
  SCHED -->|SQL| DB

  Host["Host (Docker Engine)<br/>Demo 'alpine-container-N'<br/>ports 22221..22230 -> 22"]

  API -.->|SSH to host.docker.internal:222xx| Host
  SCHED -.->|SSH to host.docker.internal:222xx| Host
```)


= Démonstration
