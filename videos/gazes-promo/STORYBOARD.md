---
format: 1920x1080
duration: 37s
message: "Un clic : Gazes cherche, choisit la meilleure version et la lance dans le navigateur"
arc: Hook → Value (footage) → Why it's strong (cherche, choisit, lance) → Proof → CTA
audience: fans d'anime francophones qui veulent lancer un épisode sans rien installer
mode: collaborative
music: energetic electronic tech launch, strong build and drop, uplifting, no vocals
---

## Video direction

- **Musique (analysée, 46 s)** : intro calme 0–8 s, montée 8–12 s, plein régime dès 12,0 s (le « drop »), énergie constante ensuite ; la pub fait 37 s, le morceau est coupé à 37 s avec un fondu sur les 2 dernières secondes. Le clic de la frame 2 tombe sur le drop (12,0 s global = 6,0 s dans la frame 2).
- **Palette** (frame.md, noms inversés par le remix, valeurs normatives) : fond #09090b, texte #fafafa, accent violet #9b8afb, gris secondaire #a1a1aa, surfaces #151517. Le jaune #FACC15 du remix est écarté. Aplats uniquement.
- **Typo** : DM Sans (titres, minuscules, graisse 650, serrés), Fraunces pour le mot « gazes » du logo, mono pour les étiquettes.
- **Grammaire de mouvement** : courbes longues (power3), pas de rebond. Une seule caméra, mouvements vers l'avant (poussées, plongées, zoom-out qui révèle) ; jamais de retour en arrière. Chaque pièce arrive quand la musique la nomme (sur le temps), jamais tout d'un coup au départ.
- **Rythme / frames tenues** : la frame 1 respire (intro calme, montée lente). La frame 5 (finale) est tenue : plus rien ne bouge, le logo et la phrase se lisent.
- **Vérité** : écrans, lecteur et affiches sont réels (pré-prod). Les scènes d'anime sont de vrais plans lus dans le lecteur Gazes (Attack on Titan, Jujutsu Kaisen, Fullmetal Alchemist). Seule la pastille de classement des versions (frame 3, beat « choisit ») est une illustration, signalée comme telle. Aucun nom d'indexeur, aucun torrent, aucun chiffre inventé, aucune promesse de délai.
- **Principe d'image** : le plein cadre est du vrai épisode (clips nets, interface rognée). Les mots géants se posent sur une plaque #09090b (aplat) pour rester lisibles sur les scènes claires. Aucune capture d'interface à l'échelle 1 : tout élément d'interface montré fait au moins un tiers du cadre.
- **Interdits** : dégradés, lueurs violettes « IA », fausse interface hors frame 4, curseur système, diaporama (tout posé puis figé) et économiseur d'écran (éléments qui flottent sans raison).

## Frame 1 — Hook

- scene: plongée dans la pupille, le mot « gazes », le mot traverse la caméra et la page se pose ; « choisis. clique. regarde. »
- duration: 6s
- transition_in: cut
- status: built
- src: compositions/frames/01-hook.html
- blueprint: zoom-out-workspace-reveal (Adapt)
- asset_candidates: assets/eye.png, assets/scroll-000.png
- focal: assets/eye.png
- roles: eye = cutout (macro d'ouverture, vraie scène du lecteur, interface rognée) · scroll-000 = background (page révélée)
- sfx: riser-soft
- narrativeRole: hook
- handoff_out: la page d'accueil est cadrée plein écran à x 0, y 0, échelle 1, opacité 1, immobile (la frame 2 démarre sur ce même cadre)

Adapt : on garde le zoom-out unique qui re-cadre le mystère ; le détail de départ est l'œil en gros plan d'une vraie scène (Attack on Titan, épisode 1), la page entière est la révélation.
Scene 1 (0.0–2.0s): fond #09090b ; le gros plan de l'œil (eye.png, vrai plan de l'épisode) occupe tout le cadre, immobile sauf une dérive très lente ; aucun texte. Plein cadre, 1 couche.
Scene 2 (2.0–4.0s): zoom-out continu qui décélère ; la page Gazes entière se révèle, nav et bouton « Regarder » visibles ; sur le temps, « choisis. » puis « clique. » arrivent en bas à gauche, très grand, minuscules, blanc. Asymétrique 70/30, 3 couches (image, voile, texte).
Scene 3 (4.0–6.0s): le zoom-out se verrouille ; « regarde. » arrive en violet sous les deux autres et tient ; le bouton « Regarder » reçoit un trait violet net (pas de lueur). Tenue de lecture, vibration très légère seulement.

## Frame 2 — Value

- scene: rafale d'affiches sur la montée, clic sur « Regarder » sur le drop, plongée dans le vrai lecteur puis plein écran sur du vrai épisode ; « un clic. gazes s'occupe du reste. »
- duration: 10s
- transition_in: cut
- status: built
- src: compositions/frames/02-value.html
- blueprint: camera-journey (Adapt)
- asset_candidates: assets/scroll-000.png, assets/poster-2.jpg, assets/poster-3.jpg, assets/poster-4.jpg, assets/poster-5.jpg, assets/poster-6.jpg, assets/player-play.mp4, assets/clip-aot.mp4
- focal: assets/clip-aot.mp4
- roles: clip-aot.mp4 = cutout (plein cadre propre, la preuve) · player-play.mp4 = supporting (lecteur avec interface, 0,8 s à l'arrivée de la plongée) · scroll-000 = background · posters = supporting
- sfx: riser, click, impact-soft, whoosh
- narrativeRole: value claim
- handoff_in: la page d'accueil plein cadre, x 0, y 0, échelle 1, opacité 1, immobile (reprend la fin de la frame 1)

Adapt : l'aller-retour d'action (plongée, clic, conséquence) est gardé ; la conséquence est maintenant longue : 3,5 s de vrai épisode plein cadre.
Scene 1 (0.0–3.0s): la page d'accueil tient, les affiches défilent en bandes de plus en plus serrées sur la montée (riser). Plein cadre, 3 couches.
Scene 2 (3.0–5.5s): la rafale s'efface d'un coup ; un point violet glisse vers « Regarder » et reste dessus.
Scene 3 (5.5–6.0s): le clic tombe sur le drop (6,0 s ici = 12,0 s global) ; le bouton s'enfonce.
Scene 4 (6.0–6.5s): plongée dans l'écran jusqu'au vrai lecteur (player-play.mp4, interface visible, 0,8 s) qui prouve que c'est le vrai produit.
Scene 5 (6.5–10.0s): coupe sur un temps fort vers le plein cadre propre clip-aot.mp4 (oies puis foule, sans interface) ; « un clic. » arrive à 6,8 s, « gazes s'occupe du reste. » à 7,6 s en violet sur « s'occupe du reste. », sur une plaque #09090b en haut à gauche, texte de plus de 100 px. Tenue en fin de frame.

## Frame 3 — Why it's strong

- scene: trois coups sur trois scènes d'anime : « cherche. », « choisit. », « lance. », chacun avec un élément réel ou illustré de Gazes en grand
- duration: 12s
- transition_in: cut
- status: built
- src: compositions/frames/03-why.html
- blueprint: kinetic-type-beats (Adapt)
- asset_candidates: assets/clip-fmab.mp4, assets/clip-jjk.mp4, assets/prep-screen.png, assets/player-still.png
- focal: assets/clip-jjk.mp4
- roles: clip-fmab.mp4 = background (beat 1) · clip-jjk.mp4 = background (beats 2 et 3, deux passages différents du clip) · prep-screen.png = supporting (beat 1, étapes réelles, grandes) · player-still.png = supporting (beat 3, bouton « Sources » du vrai lecteur)
- sfx: whoosh, tick, tick, tick, impact-soft, impact-soft
- narrativeRole: why it's strong (cherche / choisit / lance)

Adapt : la blueprint enchaîne des mots géants sur des beats ; ici chaque beat tient 4 s et porte une scène d'anime et une preuve.
Scene 1 (0.0–4.0s): « cherche. » en géant (plus de 200 px), à gauche sur plaque #09090b, sur le clip fmab (nuit bleue, plans de Paris puis cercle). À droite, grand (au moins un tiers du cadre), l'écran réel « Préparation de l'épisode » : les trois étapes « Recherche des sources… », « Vérification des fichiers… », « Préparation de la lecture… » se cochent à 1,0 s, 2,0 s, 3,0 s. Sous-titre : « des sources, pour toi. »
Scene 2 (4.0–8.0s): coupe dure sur un temps fort ; « choisit. » en géant sur le clip jjk (Itadori, lanterne dorée) ; à droite une pile de cartes de versions (VOSTFR · 720p, MULTI · 1080p, VF · 1080p) se classe, VF · 1080p monte en tête avec coche violette ; petite mention « illustration ». Sous-titre : « la meilleure version d'abord. »
Scene 3 (8.0–12.0s): coupe dure ; « lance. » en géant sur le clip jjk (Gojo puis fin de plan), à droite un recadrage du vrai lecteur avec le bouton « Sources » cerné de violet (très grand). Sous-titre : « dans ton navigateur. et si ça coince, la suivante prend le relais. » Tenue sur la dernière seconde.

## Frame 4 — Proof

- scene: mur d'affiches réelles, puis un seul écran réel en très grand : le calendrier « Cette semaine » ; « tout l'anime au même endroit. »
- duration: 5s
- transition_in: cut
- status: built
- src: compositions/frames/04-proof.html
- blueprint: grid-card-assemble (Adapt)
- asset_candidates: assets/poster-7.jpg, assets/poster-8.jpg, assets/poster-9.jpg, assets/poster-10.jpg, assets/poster-11.jpg, assets/poster-12.jpg, assets/scroll-013.png
- focal: assets/scroll-013.png
- roles: posters = supporting (mur) · scroll-013 = cutout (calendrier, recadré sur « Cette semaine », très grand)
- sfx: whoosh, riser-soft
- narrativeRole: evidence

Adapt : même cascade de cases que la blueprint, mais avec des affiches réelles, et la case finale est un vrai écran très agrandi, lisible.
Scene 1 (0.0–2.0s): les affiches s'assemblent en cascade plein cadre (6 colonnes), sur les temps forts, puis défilent.
Scene 2 (2.0–3.6s): le mur recule d'un coup (zoom-out) et le calendrier « Cette semaine » (recadré à partir de scroll-013) occupe plus de 60 % du cadre, lisible.
Scene 3 (3.6–5.0s): « tout l'anime au même endroit. » arrive en grand en bas (plus de 90 px), violet sur « au même endroit. » ; tenue.

## Frame 5 — CTA

- scene: tout se vide, le logo gazes et la phrase finale, tenue
- duration: 4s
- transition_in: cut
- status: built
- src: compositions/frames/05-cta.html
- blueprint: kinetic-type-beats (Adapt)
- asset_candidates: assets/scroll-000.png
- focal: le mot « gazes » composé en texte (Fraunces), pas de fichier logo
- roles: scroll-000 = supporting (non utilisé)
- sfx: impact-soft
- narrativeRole: cta

Le mot-marque se pose seul puis la phrase ; frame tenue. L'adresse publique est à confirmer avant le rendu ; en attendant, la frame ne l'affiche pas.
Scene 1 (0.0–1.5s): le cadre se vide jusqu'à l'aplat #09090b ; « gazes » se pose au centre, très grand (Fraunces), blanc.
Scene 2 (1.5–4.0s): « ton prochain anime t'attend. » arrive en dessous, violet sur « t'attend. » ; tenue immobile jusqu'à la fin, fondu de la musique sur les 2 dernières secondes.
