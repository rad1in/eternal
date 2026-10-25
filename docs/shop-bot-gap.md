# Telegram shop bot: gap analysis

Internal working document. The reference is the feature list of a PHP/MySQL
Telegram shop bot that a competing panel bundles. It is used as a checklist
only: nothing is copied from it (it is AGPL-3.0 and PHP), every feature below
is implemented natively in Go on our own models, runtime path and wording.

What Eternal already had before this work (verified in the code, not assumed):

- Bot: `/shop` with plans -> account -> confirm -> checkout -> orders, English-only
  panel language, `/trial` (one trial per Telegram id), client expiry and quota
  warnings DMed to the bound Telegram account, admin `/broadcast`, admin client
  search, reworded bot texts per language for a fixed key list.
- Panel and public web shop: plans, orders (state machine pending -> review ->
  paid -> fulfilled, idempotent fulfilment under `fulfilMu`), discount codes
  (percent or fixed, expiry, global use limit, once per customer, per plan),
  wallet ledger (admin credit, referral bonus, spend, refund), referral codes,
  support tickets, gateways Zarinpal, IDPay, NOWPayments, Stripe, PayPal,
  wallet and card-to-card with a typed receipt reference, reconciler job that
  settles or abandons online orders.

## Decisions

Legend: have = already in Eternal, build = implemented in this change,
skip = deliberately not built (reason given).

| Area | Reference bot does | Eternal before | Decision |
| --- | --- | --- | --- |
| Main menu | Reply/inline menu: buy, services, wallet, test, referral, support, language | Two buttons (plans, orders) | build: one inline menu, edited in place, role aware (admin row) |
| Per-user language | fa, en, ru, zh chosen by the user | One panel-wide bot language | build: `bot_users.lang`, picker over every shipped locale, Telegram `language_code` as first guess; admin text overrides honoured per language |
| Plans and categories | Categories, then products | Flat list sorted by `sort_order` | build: `plans.category`, bot shows categories only when more than one exists, plan form gets the field |
| Custom volume and days | Buyer types GB and days, price by unit price | none | build: calculator (`custom` sales config: per-GB and per-day price, bounds, template); same pricing path as plans, usable by new, renew and add-on orders, also exposed to the web shop API |
| Renew | Renew a service | Renew exists (resets usage, stacks days) | have; now reachable from "My services" too |
| Add volume / add time | Extra volume and extra time on an existing service | none | build: order kind `addon` adds GB and/or days without resetting usage; priced with the calculator |
| My services | Service list, usage, QR, link, change link | `/usage` text only, links via admin-only callbacks | build: list with usage bars, expiry, QR code, subscription link, ownership checked by Telegram id on every tap |
| Change link / location | Regenerate config, change location | Panel "regenerate sub id" exists for admins | skip: self-service regeneration breaks every config the buyer already shared and needs a node picker per inbound; add when a seller asks |
| Card to card + receipt photo | Photo receipt, admin approves or rejects with buttons | Typed reference only, approval in panel | build: photo or text receipt, forwarded to admins (and an optional group/channel) with Approve / Reject buttons that go through `OrderService`; customer is told the result |
| Gateways | Zarinpal, IDPay-like, NOWPayments, Plisio, Tron, Stars, aqayepardakht, others | Zarinpal, IDPay, NOWPayments, Stripe, PayPal, card, wallet | skip more: Plisio and the other Iranian gateways have no stable public documentation we can test against; card-to-card with admin approval already covers "any other gateway" manually. Telegram Stars skipped for now: needs a rate setting per currency and cannot be exercised without Telegram |
| Wallet top-up | Pay a deposit by any gateway | Admin credit and referral only; wallet is per service account | skip: the wallet is a per-service ledger by design (balance follows the username); a top-up kind would need a "which service" step for every buyer. Direct payment per order covers the same need |
| Discount codes | Codes with groups, per-user limit, first purchase only | Done (`discount_codes`, admin tab, bot asks the code) | have |
| Gift (balance) codes | Code that credits the wallet | none | skip: admin wallet credit does the same; a code store is a new abuse surface |
| Referral commission, welcome gift | Percent commission, welcome gift for both sides | Commission to the referrer's wallet exists, web only | build: referral link and code in the bot (`/start ref_CODE` captured, used on the first purchase); welcome gift skipped (no gift budget concept, can be a discount code) |
| Free test account | Test account with count limit | `/trial`, once per Telegram id (`client_trials`) | have; menu button added. Cooldown is moot: it is once ever |
| Forced channel / group join | Must join before use | none | build: `bot.forceJoin` list, checked with `getChatMember`, cached briefly, join buttons plus "I joined" |
| Agents / resellers | Agent role, agent price tiers, agent requests | Panel "reseller" is the hosted-panel product, not sales agents | skip: needs a price-tier model and a request workflow; no data model to reuse |
| Notifications | Low balance, expiry, payment pending, status of the day | Expiry and quota warnings to the owner chat | build: order events to the buyer (receipt under review, delivered, rejected), reminder for unpaid orders, stale unpaid card orders expire (also frees the username); expiry/quota warnings now carry a Renew button |
| Admin tools in bot | Stats of day, find user, add balance, broadcast, ban | `/broadcast`, client search | build: shop admin menu (today's sales, receipts waiting, ban/unban a Telegram id); broadcast and client search already exist; add balance stays in the panel wallet tab |
| Anti-abuse | Blocked users, spam limits | Invite-attempt limiter only | build: banned Telegram ids, per-user callback rate limit, cap on open orders per Telegram id |
| Editable texts and buttons | Admin edits menu texts and buttons | Fixed list of reword keys, bot-wide language | build: more shop keys in the same override store (now per language of the buyer) and admin-defined link buttons (support channel, tutorials) |
| Support | Ticket-like messages to admin | Tickets in panel and web shop only | build: "Support" in the bot opens and continues tickets on the same `tickets` tables; admin replies reach the buyer in Telegram |
| Mini App | Web app inside Telegram | none | build: "Open shop" WebApp button -> public shop page; server verifies Telegram `initData` (HMAC-SHA256 with the bot token, freshness window) and signs the buyer in: orders bind to the Telegram id, own services are listed, wallet and tickets need no proof |
| Invoices / receipts | Invoice message per purchase | Order message | have (the order message is the receipt); no PDF |
| Payment expiry cleanup | Cron that expires payments | Online orders only | build: same job now also expires unpaid card orders |
| Lottery, notes on services, uptime monitors, DB backup bot | Fun/ops extras | Backups and monitors exist in the panel | skip: outside a shop |

## Why this is ahead of the bundled PHP bot

- One Go binary: bot, panel and shop share one database, one settings store and
  the `runtime.Runtime` path, so a bought service lands on the right node.
- Orders stay consistent: every state change goes through `OrderService`
  (fulfilment under one mutex, wallet entries booked once by reference, receipts
  and rejections only from allowed states), whichever of web, bot or admin
  button triggers it.
- Telegram identity is verified server side (bot callbacks carry the real
  sender; Mini App `initData` is HMAC checked), never trusted from the client.

## Not verifiable without real services

Real Telegram (long polling, photos, `getChatMember`, WebApp launch), real
gateways and real payments cannot run here. The bot is driven against a
fake Bot API server and a throwaway database; the Mini App check is covered by
test vectors computed with the documented algorithm.
