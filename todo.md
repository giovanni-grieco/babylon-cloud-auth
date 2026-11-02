- chiedere a max cosa si intende per OAuth2.0 -- FATTO -- prima il token era opaco e non conteneva informazioni. ogni volta che si voleva conoscere l'ID dell'utente si doveva interrogare il DB con questa token identificativo. Adesso invece l'ID dell'utente è direttamente contenuto nel JWT. Quindi era per motivi di prestazioni (troppe chiamate concorrenti al DB) e per omologazione (compliance)
- implementazione JWT completa (access token, refresh token, firma con chiavi asimmetriche) -- FATTO
- Implementazione claim in JWT per due servizi (tipo a e b) 
- Creare servizio A e B per verificare i diversi permessi utente. Utente 1 può usare solo il servizio A ma non il B.
- implementazione di bloom filter per home-realm-discovery
- Implementare sistema di identificazione utenti con chiavi non progressive ma comunque ordinabili (uuid potrebbe essere utile in qualche modo)

==== NUOVO ====

Utilizziamo Keycloak e OAuth2.0 proxy come middleware the autenticazione che implementano OpenID Connect
- Capire a livello applicativo che informazioni vede il mock-svc 
- Creare un portale di provisioning dove si decide l'username, il realm su cui cadere e la prima password etc.
- Dopo il provisioning, se un servizio vede l'utente per la prima volta, crea autorizzazioni di default e le salva nel suo DB
- Creare un flusso di comunicazione per unprovisioning e cancellare tutte le risorse e informazioni di autorizzazione relative all'utente cancellato

Nel futuro:
Creiamo un provisioning-svc che consente ad utenti di registrarsi direttamente senza che un admin abbia creato le loro credenziali direttamente.