// Editorial content of the Business page (from the design): revenue ideas and risks.
// These are hypotheses to validate, never figures: nothing here is measured, nothing is promised.

export interface RevenueIdea {
  title: string;
  idea: string;
  measures: string[];
  caveat: string;
}

export const REVENUE_IDEAS: RevenueIdea[] = [
  {
    title: "Dons",
    idea: "Proposer un lien de soutien discret pour couvrir les coûts d'infrastructure, sans rien retirer au service gratuit.",
    measures: [
      "Part des actifs qui reviennent régulièrement (historique des séances de visionnage)",
      "Clics sur le lien de soutien et pages vues du lien [À MESURER]",
      "Don moyen et nombre de donateurs [À MESURER]",
    ],
    caveat: "À tester d'abord avec un lien sans paiement intégré, pour mesurer l'intention.",
  },
  {
    title: "Offre de confort sans publicité",
    idea: "Vérifier si une expérience plus calme (sans bandeaux ni sollicitations) a une valeur pour les actifs réguliers.",
    measures: [
      "Ce qui gêne aujourd'hui les spectateurs : sondage court [À MESURER]",
      "Part d'actifs avec un compte et un historique de visionnage",
      "Intentions d'achat à [TARIF] via une page d'attente [À MESURER]",
    ],
    caveat: "Dépend de l'existence d'une gêne réelle à retirer : rien ne l'établit aujourd'hui.",
  },
  {
    title: "Stockage AV1 prioritaire",
    idea: "Réserver la conversion et la conservation de copies AV1 aux comptes qui soutiennent le service.",
    measures: [
      "Part des visionnages servis depuis une copie AV1 [À MESURER]",
      "Économie de bande passante par heure en AV1 par rapport à la source [À MESURER]",
      "Demandes de conversion refusées faute de place [À MESURER]",
    ],
    caveat: "À valider aussi côté coût : le calcul AV1 consomme du processeur et du disque.",
  },
];

export interface Risk {
  level: "haute" | "moyenne";
  title: string;
  text: string;
  signal: string;
  mitigation: string;
}

export const RISKS: Risk[] = [
  {
    level: "haute",
    title: "Dépendance aux sources externes",
    text: "Le catalogue repose sur des sources et des trackers tiers : si elles disparaissent ou changent, la lecture s'arrête.",
    signal: "Échecs par source et codes d'erreur de lecture en hausse (voir Lecteur et flux).",
    mitigation: "Multiplier les sources par épisode et conserver des copies locales des épisodes les plus regardés.",
  },
  {
    level: "haute",
    title: "Juridique",
    text: "La diffusion d'œuvres via BitTorrent expose à des demandes de retrait et à des mesures de l'hébergeur. Aucun avis juridique n'est joint à ce panel.",
    signal: "Courriers de retrait, avertissements de l'hébergeur, changement de conditions du fournisseur.",
    mitigation: "Obtenir un avis juridique avant tout revenu : [À VALIDER avec un conseil]. Prévoir une procédure de retrait.",
  },
  {
    level: "moyenne",
    title: "Saturation de la machine",
    text: "Une seule machine sert la lecture, le remux et le stockage : au-delà de sa limite, la lecture se dégrade pour tous.",
    signal: "Pic de flux au-dessus de 80 % de la limite (une fois celle-ci mesurée), démarrage de lecture de plus en plus lent (voir Lecteur et flux).",
    mitigation: "Test de charge pour fixer la limite, puis préparer une seconde machine avant d'atteindre la saturation projetée.",
  },
];
