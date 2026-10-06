"use client";

import { useSyncExternalStore } from "react";
export type Locale = "fr" | "en";
const eventName = "gazes-language-change";
let memoryLocale: Locale | undefined;

export function getLocale(): Locale {
  if (typeof window === "undefined") return "fr";
  try { const saved = localStorage.getItem("gazes-language"); if (saved === "fr" || saved === "en") return saved; } catch {}
  if (memoryLocale) return memoryLocale;
  return "fr";
}
function subscribe(callback: () => void) {
  window.addEventListener(eventName, callback);
  window.addEventListener("storage", callback);
  window.addEventListener("languagechange", callback);
  return () => {window.removeEventListener(eventName, callback); window.removeEventListener("storage", callback); window.removeEventListener("languagechange", callback);};
}
export function setLocale(locale: Locale) {
  memoryLocale = locale;
  try {localStorage.setItem("gazes-language", locale);} catch {}
  document.documentElement.lang = locale;
  window.dispatchEvent(new Event(eventName));
}

// Source labels are stable translation keys, shared by current and legacy views.
const pairs: [string, string][] = [
 ["Qualité","Quality"], ["Choisir la qualité","Choose quality"], ["Une seule qualité disponible","Only one quality available"], ["Original","Original"],
 ["Fermer la recherche", "Close search"],
 ["Reprendre", "Resume"], ["Date à confirmer", "Date to be confirmed"], ["Aperçu indisponible", "Preview unavailable"],
 ["Recherche partielle : certains résultats du fournisseur sont indisponibles ou limités.","Partial search: some provider results are unavailable or limited."],
 ["Sous-titres anglais","English-Translated"],["Sous-titres non anglais","Non-English Subbed"],["Animes non traduits","Raw Anime"],
 ["épisode","episode"],["ep","ep"],["Saison 1 (","Season 1 ("],
 ["seeders","seeds"],["pairs)","peers)"],["Toutes (","All ("],["FR (","FR ("],
 ["Tous les animes","All Anime"],["Sous-titres anglais","English-translated"],["Non traduit","Raw"],
 ["Analyse…","Detecting..."],["Voir moins","Show less"],["Lire la suite","Read more"],["Principale","Main"],["{count} ép.","{count} eps"],
 ["Catalogue","Catalog"],["Chargement…","Loading…"],["Chargement du catalogue","Loading catalog"],
 ["Réessayer","Retry"],["Fermer","Close"],["Regarder","Watch now"],["En savoir plus","Learn more"],
 ["Rechercher","Search"],["Rechercher un anime","Search anime"],["Rechercher un anime…","Search anime…"],
 ["Navigation principale","Main navigation"],["Navigation de pied de page","Footer navigation"],
 ["Gazes, accueil","Gazes, home"],["Haut de page","Back to top"],["Changer de thème clair/sombre","Toggle light/dark theme"],
 ["Langue","Language"],["Nouveautés","What’s new"],["Nouveau","New"],["Journal de développement","Dev log"],["Fermer l’annonce","Dismiss announcement"],["Tous droits réservés.","All rights reserved."],
 ["Gazes. Tous droits réservés.","Gazes. All rights reserved."],
 ["Gazes n’héberge pas de vidéos de façon permanente. Les flux proviennent de sources externes.","Gazes does not permanently host videos. Streams come from external sources."],
 ["Gazes — Découvrez votre prochain anime","Gazes — Discover your next anime"],
 ["Découvrez les animes du moment, explorez leurs saisons et regardez vos épisodes sur Gazes.","Discover current anime, explore their seasons and watch your episodes on Gazes."],
 ["Cette saison","This season"],["Les incontournables","All-time favorites"],["Suggestions","Suggestions"],["Filtrer par genre","Filter by genre"],["Clic gauche : inclure · Clic droit ou appui long : exclure","Left click: include · Right click or long press: exclude"],["{genre}, inclus","{genre}, included"],["{genre}, exclu","{genre}, excluded"],["sans {genres}","without {genres}"],["Réincarnation","Reincarnation"],["Magie","Magic"],["École","School"],["Effacer la recherche","Clear search"],["Genres","Genres"],["Effacer","Clear"],["Appliquer","Apply"],["Aventure","Adventure"],["Comédie","Comedy"],["Drame","Drama"],["Fantastique","Fantasy"],["Horreur","Horror"],["Musique","Music"],["Mystère","Mystery"],["Psychologique","Psychological"],["Science-fiction","Science fiction"],["Tranche de vie","Slice of life"],["Sport","Sports"],["Surnaturel","Supernatural"],["Confidentialité","Privacy"],["Exporter mes données","Export my data"],["Effacer mon journal","Erase my log"],["Impossible d’exporter vos données pour le moment.","Unable to export your data right now."],["Impossible d’effacer vos données pour le moment.","Unable to erase your data right now."],["Effacer votre journal de visionnage et vos « pas intéressé » ? Vos suggestions repartiront de zéro. Cette action est définitive.","Erase your watch log and your “not interested” list? Your suggestions will start over. This cannot be undone."],["Pas intéressé","Not interested"],["Pas intéressé par {title}","Not interested in {title}"],["Pour vous","For you"],["Hiver","Winter"],["Printemps","Spring"],["Été","Summer"],["Automne","Autumn"],
 ["Animes à découvrir","Discover anime"],["À la une","Featured anime"],["Aucun anime trouvé.","No anime found."],
 ["Source actuelle","Current source"],["Recherche partielle : certaines sources peuvent manquer.","Partial search: some sources may be missing."],
 
 ["Anime précédent","Previous anime"],["Anime suivant","Next anime"],
 ["Reprendre le défilement","Resume slideshow"],["Mettre le défilement en pause","Pause slideshow"],
 ["Explorer les animes","Explore anime"],["Résultats pour « {query} »","Results for “{query}”"],
 ["Impossible de charger le catalogue.","Unable to load the catalog."],["Impossible de charger les données.","Unable to load the data."],
 ["Impossible de charger les sorties de cette saison.","Unable to load this season’s releases."],
 ["Pagination","Pagination"],["Précédent","Previous"],["Suivant","Next"],["Page","Page"],
 ["Disponibilité à confirmer","Availability to be confirmed"],["Épisode 1 · Date à confirmer","Episode 1 · Date to be confirmed"],
 ["Épisode 1 ·","Episode 1 ·"],["Prévu en {date}","Expected in {date}"],
 ["{count} épisode disponible","{count} episode available"],["{count} épisodes disponibles","{count} episodes available"],
 ["{count} épisode","{count} episode"],["{count} épisodes","{count} episodes"],
 ["Bientôt disponible","Coming soon"],["Fil d’Ariane","Breadcrumb"],["Anime","Anime"],["Saison","Season"],
 ["Saisons","Seasons"],["Films","Movies"],["Spéciaux et histoires annexes","Specials and side stories"],
 ["Voir les saisons","View seasons"],["Regarder l’épisode","Watch episode"],["Tous les épisodes","All episodes"],
 ["Épisodes","Episodes"],["En cours de lecture","Now playing"],["Se connecter","Sign in"],["Copier","Copy"],["Copié","Copied"],["Code d’erreur","Error code"],["Connexion au swarm…","Connecting to the swarm…"],["Démarrage du flux…","Starting the stream…"],["Peu de pairs répondent pour l’instant, la lecture démarrera dès que possible.","Few peers are responding right now, playback will start as soon as possible."],["Le premier chargement peut prendre quelques secondes","The first load can take a few seconds"],["Récupération des métadonnées et des premières pièces","Fetching metadata and the first pieces"],["Recherche des sources…","Looking for sources…"],["Vérification des fichiers…","Checking the files…"],["Préparation de la lecture…","Getting playback ready…"],["Préparation de l’épisode","Getting the episode ready"],["La première recherche peut prendre une vingtaine de secondes. Les suivantes seront instantanées.","The first search can take about twenty seconds. The next ones will be instant."],["Calendrier","Calendar"],["Cette semaine","This week"],["Semaine du","Week of"],["Affichage","View"],["Semaine","Week"],["Mois","Month"],["Liste","List"],["Période précédente","Previous period"],["Période suivante","Next period"],["Aujourd’hui","Today"],["Jours","Days"],["Aucune sortie","No releases"],["Aucune sortie ce jour-là.","No releases that day."],["{count} sortie","{count} release"],["{count} sorties","{count} releases"],["Diffusé","Aired"],["Ce soir","Tonight"],["Impossible de charger le calendrier des sorties.","Unable to load the release calendar."],["Certaines sorties n’ont pas pu être chargées.","Some releases could not be loaded."],["Source indisponible","Source unavailable"],["Cette source ne répond pas.","This source is not responding."],["Nous essayons automatiquement la suivante, sans perdre votre position.","We are automatically trying the next one, without losing your position."],["Sans réponse","No response"],["Changer de source","Change source"],["Créez un compte pour retrouver votre historique et reprendre là où vous vous êtes arrêté, sur tous vos appareils.","Create an account to keep your history and pick up where you left off on all your devices."],["S’inscrire","Sign up"],["Compte","Account"],["Panneau d’administration","Admin panel"],["Historique","History"],["Se déconnecter","Sign out"],["Connexion","Sign in"],["Inscription","Sign up"],["Créer un compte","Create an account"],["Créer mon compte","Create my account"],["Adresse e-mail","Email address"],["Mot de passe","Password"],["Pseudo","Username"],["votrepseudo","yourusername"],["vous@exemple.com","you@example.com"],["Afficher le mot de passe","Show password"],["Masquer le mot de passe","Hide password"],["8 caractères minimum.","8 characters minimum."],["J’accepte les conditions d’utilisation","I accept the terms of use"],["Déjà un compte ?","Already have an account?"],["Pas encore de compte ?","No account yet?"],["Un instant…","One moment…"],["Vérification automatique à l’envoi, sans case à cocher.","Automatic check when you submit, no checkbox."],["Vérification…","Verifying…"],["Vérifié","Verified"],["Échec de la vérification. Réessayer","Verification failed. Try again"],["Gardez votre progression partout.","Keep your progress everywhere."],["Reprenez là où vous vous êtes arrêté.","Pick up where you left off."],["Un compte, c’est votre historique de lecture sur tous vos appareils.","An account keeps your watch history on all your devices."],["Votre progression est enregistrée et vous suit d’un appareil à l’autre.","Your progress is saved and follows you from device to device."],["Titre","Title"],["Reprendre la lecture","Resume watching"],["Rien à reprendre pour l’instant.","Nothing to resume yet."],["Adresse e-mail invalide.","Invalid email address."],["Le pseudo doit faire 3 à 24 caractères (lettres, chiffres, _ . -).","The username must be 3 to 24 characters (letters, digits, _ . -)."],["Le mot de passe doit faire au moins 8 caractères.","The password must be at least 8 characters."],["E-mail ou mot de passe incorrect.","Incorrect email or password."],["Cette adresse e-mail est déjà utilisée.","This email address is already in use."],["Vérification anti-robot échouée. Réessayez.","Anti-bot check failed. Try again."],["Trop de tentatives. Réessayez dans quelques minutes.","Too many attempts. Try again in a few minutes."],["Impossible de joindre le serveur.","Cannot reach the server."],["Vous devez accepter les conditions d’utilisation.","You must accept the terms of use."],["Confirmez que vous n’êtes pas un robot.","Please confirm you are not a robot."],["Une erreur est survenue. Réessayez.","Something went wrong. Try again."],["Ambilight","Ambilight"],["Réglages","Settings"],["Lueur autour de la vidéo","Soft glow around the video"],["Intensité","Intensity"],["Douce","Soft"],["Moyenne","Medium"],["Vive","Vivid"],["Atténuer en pause","Dim when paused"],["Fondu après 3 s","Fades after 3 s"],["Désactivé automatiquement en plein écran.","Turns off automatically in fullscreen."],["Épisodes précédents","Previous episodes"],["Épisodes suivants","Next episodes"],["Épisode","Episode"],["À venir","Coming soon"],["· À venir","· Coming soon"],
 ["Date inconnue","Unknown date"],["Nombre d’épisodes inconnu","Episode count unknown"],
 ["La liste des épisodes n’est pas encore disponible.","The episode list is not available yet."],
 ["La meilleure source est choisie automatiquement. Si elle ne démarre pas ou se bloque, la suivante est essayée.","The best source is selected automatically. If it fails to start or stalls, the next one is tried."],
 ["Certaines informations de saisons sont temporairement indisponibles.","Some season information is temporarily unavailable."],
 ["Toutes les tentatives de lecture ont échoué.","All playback attempts failed."], ["La recherche de sources a échoué.","The source search failed."],
 ["La recherche de torrents est incomplète. Réessayez.","The torrent search is incomplete. Please try again."],
 ["Aucun torrent ne correspond à cet épisode.","No torrent matches this episode."],
 ["Les fournisseurs de torrents sont temporairement indisponibles.","Torrent providers are temporarily unavailable."],
 ["Référence de diagnostic","Diagnostic reference"],
 ["Aucune source disponible ne permet de lire cet épisode.","None of the available sources can play this episode."],
 ["Source automatique {current}/{total}","Automatic source {current}/{total}"],
 [" · La précédente était indisponible"," · The previous source was unavailable"],
 ["Fermer le lecteur","Close player"],["Changer de source","Change source"],["Sources","Sources"],
 ["L’épisode demandé n’est pas identifié sans ambiguïté dans ce pack.","The requested episode cannot be identified unambiguously in this pack."],
 ["Cette source ne fournit pas ses métadonnées à temps.","This source did not provide its metadata in time."],
 ["La lecture ne démarre pas sur cette source.","Playback does not start on this source."],
 ["Cette source ne fournit plus de vidéo.","This source stopped providing video."],
 ["Impossible de reprendre la lecture. Cliquez sur Lecture pour réessayer.","Unable to resume playback. Click Play to try again."],
 ["Ce navigateur ne prend pas en charge la vidéo H.265/HEVC. Choisissez une source H.264/AVC ou un navigateur compatible HEVC.","This browser does not support H.265/HEVC video. Choose an H.264/AVC source or an HEVC-compatible browser."],
 ["Cette vidéo H.265/HEVC ne peut pas être décodée dans ce navigateur ou sur cet appareil. Essayez une source H.264/AVC ou un navigateur compatible HEVC.","This H.265/HEVC video cannot be decoded by this browser or device. Try an H.264/AVC source or an HEVC-compatible browser."],
 ["Impossible de décoder cette vidéo. Essayez une autre source ou un navigateur compatible avec son codec.","Unable to decode this video. Try another source or a browser that supports its codec."],
 ["La lecture a été interrompue. Réessayez ou choisissez une autre source.","Playback was interrupted. Retry or choose another source."],
 ["La lecture de cette source s’est interrompue avant la fin de l’épisode.","This source stopped playing before the episode ended."],
 ["Impossible de charger les sous-titres. Désactivez puis resélectionnez la piste pour réessayer.","Unable to load subtitles. Turn subtitles off and reselect the track to try again."],
 ["Impossible d’afficher les sous-titres.","Unable to display subtitles."],
 ["Cet épisode ne peut pas être identifié dans ce pack. Essai de la source suivante…","This episode cannot be identified in this pack. Trying the next source…"],
 ["Choisissez le fichier correspondant à l’épisode {episode}. Aucun fichier n’a été lancé automatiquement.","Choose the file for episode {episode}. No file was started automatically."],
 ["Rechercher un fichier","Search files"],["Rechercher un fichier…","Search files…"],
 ["fichiers","files"],[" correspondant à cet épisode"," matching this episode"],[" vidéo"," video"],
 ["Affichage des 50 premiers fichiers. Affinez la recherche.","Showing the first 50 files. Refine your search."],
 ["Connexion à l’essaim BitTorrent…","Connecting to BitTorrent swarm..."],
 ["Récupération des métadonnées et des morceaux séquentiels","Fetching metadata & sequential pieces"],
 ["L’essaim peut ne pas avoir de seeders actifs ou la connexion au tracker a expiré.","The torrent swarm may have no active seeders or tracker connection timed out."],
 ["Votre navigateur ne prend pas en charge la lecture vidéo HTML5.","Your browser does not support HTML5 video playback."],
 ["Mise en mémoire tampon…","Buffering stream..."],["Chargement du flux…","Buffering..."],
 ["Épisode précédent","Previous Episode"],["Épisode suivant","Next Episode"],["Suivant","Next"],["Bibliothèque","Library"],["Masquer","Hide"],["Votre liste est vide. Ajoutez des animes depuis leur fiche ou le calendrier.","Your list is empty. Add anime from their page or the calendar."],["Lecture","Play"],["Affiner la recherche","Refine search"],["Recherche","Search"],["Tous les animes","All anime"],["Aucun anime ne correspond à cette recherche.","No anime matches this search."],["Effacer les filtres","Clear filters"],["Remonter d’un niveau","Go up one level"],["Afficher cette aide","Show this help"],["Raccourcis clavier","Keyboard shortcuts"],["Résultats","Results"],["Vos animes","Your anime"],["Recherches récentes","Recent searches"],["Ajouter {title} à ma liste","Add {title} to my list"],["Lire l’épisode 1 de {title}","Play episode 1 of {title}"],
 ["Passer l'opening","Skip opening"],["Passer l'ending","Skip ending"],
 ["Reculer de 10 s (←)","Rewind 10s (←)"],["Avancer de 10 s (→)","Forward 10s (→)"],
 ["Couper le son (M)","Mute (M)"],["Rétablir le son (M)","Unmute (M)"],
 ["Pause (Espace)","Pause (Space)"],["Lecture (Espace)","Play (Space)"],["Volume","Volume"],
 ["Choisir la piste audio","Select Audio Track"],["Pistes audio","Audio Tracks"],["Audio par défaut","Default Audio"],
 ["Audio","Audio"],["Choisir les sous-titres","Select Subtitles"],["Sous-titres","Subtitles"],["Désactivés","Off"],
 ["Plein écran (F)","Toggle Fullscreen (F)"],["Seeders","Seeders"],[" pairs)"," peers)"],
 ["Vitesse de téléchargement","Download Speed"],["Durée","Duration"],["Taille du fichier","File Size"],
 ["Épisode / Fichier (","Episode / File ("],["éléments) :","items):"],
 ["Forcer le remux FFmpeg (conversion audio AAC stéréo)","Force FFmpeg Remux (Stereo AAC Transcoding)"],
 ["Lien magnet copié","Magnet Copied"],["Copier le lien magnet","Copy Magnet Link"],
 ["Changer de source pour cet épisode","Change source for this episode"],
 ["Chargement des détails et des saisons…","Loading anime details & seasons..."],
 ["Lecture de l’épisode 1","Play Episode 1"],["SAISONS :","SAISONS:"],
 ["Aucun épisode répertorié pour cet anime.","No episodes listed for this anime."],
 ["Lecture directe","Direct Stream"],["Trier :","Sort:"],["Plus récents","Latest"],["Taille","Size"],
 ["Rechercher un épisode ou un numéro…","Search episode or #..."],["Aller à :","Jump to:"],
 ["Aucun épisode ne correspond à votre recherche.","No episode matched your search query."],
 ["Recherche de sources…","Finding sources..."],["Lire","Play"],["sur","of"],
 ["Voir toutes les sources et résolutions disponibles","Browse all available sources / resolutions"],
 ["sources disponibles","sources available"],["avec sous-titres ou audio français)","with French sub/dub)"],
 ["Langue :","Lang:"],["Toutes les qualités","All Qualities"],["Filtrer les releases…","Filter releases..."],
 ["Recherche de sources pour l’épisode","Searching sources for episode"],
 ["Aucune source ne correspond à vos critères.","No sources found matching your criteria."],
 ["Diffuser","Stream"],["Toutes les équipes","All Groups"],["Rechercher une release…","Search release..."],
 ["Rechercher un anime (ex. Frieren, Dandadan, One Piece)…","Search anime (e.g. Frieren, Dandadan, One Piece)..."],
 ["Moteur BitTorrent","Swarm Engine"],["Releases","Releases"],
 ["À propos de la saison","About this season"],["Année","Year"],["Statut","Status"],["En cours","Ongoing"],["Depuis le début","From the start"],["Les saisons","Seasons"],["Retour à la série","Back to the series"],["Dans cette série","In this series"],["{count} saisons","{count} seasons"],["{count} saison","{count} season"],["En cours de visionnage","In progress"],["Aperçu enregistré","Preview saved"],
 ["Genre","Genre"],["Genres","Genres"],["Animes {genre} en streaming","{genre} anime streaming"],
 ["Copier le lien à cet instant","Copy link at this time"],["Lien copié","Link copied"],
 ["Épisode suivant dans {seconds} s","Next episode in {seconds}s"],["Lancer maintenant","Play now"],["Annuler","Cancel"],
 ["Tout voir","See all"],["Ma liste","My list"],["Regarder ensemble : copier l’invitation","Watch together: copy invite"],["Ajouter les sorties à mon calendrier","Add releases to my calendar"],["Aucune sortie prévue dans les 6 prochaines semaines.","No releases scheduled in the next 6 weeks."],["Impossible de générer le calendrier pour l’instant.","Could not build the calendar right now."],["Dans ma liste","In my list"],["À voir plus tard","Watch later"],["Retirer {title} de ma liste","Remove {title} from my list"],["Retirer de ma liste","Remove from my list"],["Navigation principale","Main navigation"],
 ["Soutenir Gazes","Support Gazes"],["Soutenir","Support"],["Aidez à garder Gazes en ligne","Help keep Gazes online"],["Gazes est gratuit et sans publicité. Les dons servent uniquement à payer le serveur et la bande passante. Rien n’est retiré à ceux qui ne donnent pas.","Gazes is free and ad-free. Donations only pay for the server and bandwidth. Nothing is taken away from those who do not donate."],["Impossible de charger la page de dons. Réessayez plus tard.","Could not load the donation page. Try again later."],["Bientôt","Coming soon"],["Les dons ne sont pas encore ouverts. Merci de l’intention : repassez dans quelques jours.","Donations are not open yet. Thanks for the thought: check back in a few days."],["Coûts du mois","This month’s costs"],["Part des coûts du mois couverte","Share of this month’s costs covered"],["{raised} sur {goal} couverts ce mois-ci","{raised} of {goal} covered this month"],["Donner en crypto","Donate with crypto"],["Montant","Amount"],["Autre","Other"],["Montant en euros","Amount in euros"],["Entre {min} et {max}.","Between {min} and {max}."],["Votre nom sur le mur des donateurs","Your name on the donor wall"],["Rester anonyme","Stay anonymous"],["(par défaut)","(default)"],["Personne ne voit votre don.","Nobody sees your donation."],["Afficher un nom","Show a name"],["Le nom que vous choisissez apparaît ci-dessous après le paiement.","The name you choose appears below once the payment is confirmed."],["Nom affiché","Displayed name"],["Ce nom n’est pas valide : 2 à 24 lettres, chiffres, espaces ou - _ ’, sans lien.","This name is not valid: 2 to 24 letters, digits, spaces or - _ ’, no links."],["2 à 24 lettres, chiffres, espaces ou - _ ’, sans lien.","2 to 24 letters, digits, spaces or - _ ’, no links."],["Ce don sera rattaché à votre compte, visible uniquement par l’administrateur.","This donation will be linked to your account, visible only to the administrator."],["Donner {amount} en crypto","Donate {amount} with crypto"],["Choisissez un montant","Choose an amount"],["Donner sur Ko-fi","Donate on Ko-fi"],["Ce qui est enregistré","What is recorded"],["Le montant, la date, le moyen de paiement et la référence de la transaction : visibles uniquement par l’administrateur.","The amount, date, payment method and transaction reference: visible only to the administrator."],["Si vous êtes connecté, le don est rattaché à votre compte pour que l’administrateur puisse vous remercier.","If you are signed in, the donation is linked to your account so the administrator can thank you."],["Jamais votre e-mail ni vos coordonnées bancaires. Le paiement crypto ne demande aucun compte.","Never your e-mail or bank details. Crypto payment needs no account."],["Publiquement : seulement le nom que vous avez choisi d’afficher, jamais le montant.","Publicly: only the name you chose to show, never the amount."],["Merci à","Thanks to"],["Les premiers noms apparaîtront ici. La plupart des donateurs restent anonymes, et c’est très bien.","The first names will appear here. Most donors stay anonymous, and that is perfectly fine."],["Merci","Thank you"],["Votre don est bien parti. Le réseau crypto peut mettre quelques minutes à le confirmer : si vous avez choisi d’afficher un nom, il apparaîtra sur la page de soutien une fois le paiement confirmé.","Your donation is on its way. The crypto network may take a few minutes to confirm it: if you chose to show a name, it will appear on the support page once the payment is confirmed."],["Retour au catalogue","Back to the catalog"],["Voir le mur des donateurs","See the donor wall"],["Montant invalide.","Invalid amount."],["Les dons ne sont pas encore ouverts.","Donations are not open yet."],["Le paiement crypto est momentanément indisponible. Réessayez dans quelques minutes.","Crypto payment is temporarily unavailable. Try again in a few minutes."],["Trop de tentatives. Réessayez plus tard.","Too many attempts. Try again later."],
];
const dictionary = new Map<string, { fr: string; en: string }>();
for (const [fr, en] of pairs) {const value = {fr,en};dictionary.set(fr,value);dictionary.set(en,value);}

export function translate(locale: Locale, key: string, params: Record<string, string | number> = {}): string {
  if (/^(Failed to (fetch|load)|failed to (load|get)|Unexpected token)/i.test(key)) key = "Impossible de charger les données.";
  let result = dictionary.get(key)?.[locale] || key;
  if (locale === "en") result = result.replace(/^Saison (\d+)(?: · Partie (\d+)| · Parties (\d+) et (\d+))?(?= — |$)/, (_, n, part, a, b) => `Season ${n}${part ? ` · Part ${part}` : a ? ` · Parts ${a} & ${b}` : ""}`).replace(/^Épisode (\d+)$/, "Episode $1");
  if (locale === "fr") result = result.replace(/^Season (\d+)$/, "Saison $1").replace(/^Episode (\d+)$/, "Épisode $1");
  return result.replace(/\{(\w+)\}/g, (match, name) => String(params[name] ?? match));
}
export function useI18n() {
  const locale = useSyncExternalStore(subscribe, getLocale, () => "fr" as Locale);
  return { locale, t: (key: string, params?: Record<string,string|number>) => translate(locale,key,params) };
}
