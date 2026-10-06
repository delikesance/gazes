# Dons

Objectif : couvrir les coûts d'hébergement. L'admin voit **exactement qui donne quoi** ; le public ne voit
que ce que le donateur a choisi (rien par défaut).

## Modèle de confidentialité

| Donnée | Admin | Public |
|---|---|---|
| Compte lié (pseudo, id), libellé du donateur (nom Ko-fi), référence fournisseur, montant, message | oui | non |
| Nom affiché | oui | seulement si le donateur a choisi « afficher un nom » |
| Total du mois / objectif | oui | oui, uniquement si un objectif est configuré |
| E-mail fourni par Ko-fi | jamais stocké | jamais |

Visibilité : `anonymous` (défaut) ou `named` (nom choisi, 2-24 caractères, lettres/chiffres/espace/`-_'`).
L'admin peut repasser un don en anonyme (modération) et rattacher un don à un compte.

## Fournisseurs

- **BTCPay Server** (crypto, auto-hébergé, sans compte tiers) : `POST /api/v1/donations/btcpay/invoice`
  crée une facture ; le webhook `InvoiceSettled` est signé (`BTCPay-Sig`, HMAC-SHA256) et le montant est
  relu via l'API BTCPay avant d'enregistrer (le webhook n'est jamais cru sur parole).
- **Ko-fi** : webhook `POST /api/v1/donations/webhooks/kofi` (formulaire `data`, `verification_token`),
  dédoublonné par `kofi_transaction_id`. Un don public (`is_public`) devient `named`.
- **Manuel** (admin) : virement, Liberapay, espèces.

## Pages

- `/soutenir` (public) : pourquoi, barre d'objectif (si configuré), montants 3/5/10/20 € + libre,
  choix anonyme/nom, bouton crypto, lien Ko-fi, mur des donateurs, rappel de ce qui est enregistré.
- `/soutenir/merci` : retour de paiement, confirmation asynchrone.
- `/admin/donations` (session admin uniquement, jamais un token MCP) : KPI (mois, total, nombre, don
  moyen), table des dons avec pseudo/libellé, actions (lier à un compte, masquer le nom), saisie manuelle.

## Configuration (toutes optionnelles : sans elles, la page affiche « bientôt »)

| Variable | Rôle |
|---|---|
| `GAZES_SITE_URL` | URL publique, pour la redirection après paiement |
| `GAZES_BTCPAY_URL`, `GAZES_BTCPAY_STORE_ID`, `GAZES_BTCPAY_API_KEY`, `GAZES_BTCPAY_WEBHOOK_SECRET` | BTCPay |
| `GAZES_KOFI_URL`, `GAZES_KOFI_TOKEN` | lien public et jeton de vérification du webhook Ko-fi |
| `GAZES_DONATION_GOAL_EUR` | objectif mensuel affiché (€) |

Stockage : `$ACCOUNTS_DIR/donations.sqlite` (migrations `dbmigrate`).
