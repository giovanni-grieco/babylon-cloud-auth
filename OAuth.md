# OAuth2.0, JWT
Dobbiamo stabilire dei ruoli e delle utenze.

Per il nostro caso diciamo che un utente si possa registrare, possa fare il login etc.

Quando viene fatto il login, viene generata una sessione (associata ad un refresh-token e access token).
La sessione può venire revocata dall'utente e questo dovrebbe fare il "wipe" dei cookie sul browser utente, perché tanto non funzionerebbero più.

A questo punto il flusso dovrebbe essere lo stesso. Cioè la redirezione dell'utente dal login alla pagina dell'operazione effettiva.

Il JWT ha un header (dove si dice che algoritmo di crittografia è stato usato), il payload che contiene i claims con i vari access-token e refresh-token, il contenuto crittografato dell'header+payload con chiave privata.

Chiunque voglia verificare il JWT token, deve prendere la chiave pubblica di chi l'ha firmata e decodificare l'ultima parte e vedere se la firma è valida.
Questo ci consente di verificare che l'autore del messaggio è stato l'authorization server.

# Flusso
Nel nostro caso, Traefik, potrebbe contattare l'erogatore stesso del token, perché /verify è comunque un auth-svc endpoint. Ma il token in realtà potrebbe essere verificato da qualunque servizio. Questo è un dettaglio implementativo ed è una scelta fatta per l'utilizzo di Traefik, ma ciò non è necessario con altri API gateway. Potenzialmente, ogni servizio potrebbe includere la logica di verifica del JWT.

Ad ogni caso, anche se il token è verificato, 

# Verifier Service
La responsabilità di verificare il token giace sopra il servizio di autenticazione, che viene interpellato ogni volta su /verify per verificare che il token sia apposto.

Non è troppo male come cosa, però in teoria un token stateless dovrebbe essere verificato da qualsiasi nodo, anche l'API Gateway stesso. In questo caso non è così.

# Centralizzazione
Ci sono alcuni "vocaboli" condivisi tra servizi tipo i ruoli "admin/user" etc. mentre ci sono cose più specifiche che possono essere contenute nel claim e che poi il servizio per conto suo deve verificare.

Se l'auth-svc viene levato dal cluster di lavoro, e viene allocato in un cluster "suo" allora come posso ridezionare il traffico??? Non è pù l'api gateway a orchestrare come fa adesso.

Il verifier deve creare lui stesso l'associazione state-token=redirect URI e annunciarla al servizio di autenticazione centralizzato. A questo punto, farà la redirect dello user-agent verso il servizio di autenticazione che a questo punto ha già avuto l'informazione di quello stato, e procederà con il flusso normale finendo con la classica re-direzione all'URL originale. Questo è necessario a meno che, non codifichiamo delle informazioni all'interno della stringa che dovrebbe rimanere opaca al client. Quindi possiamo utilizzare crittografia asimmetrica per fare ciò. Dove sia il verifier che gli auth-svc hanno delle chiavi private e pubbliche disponibili con le quali possono firmare cose.

# Integrazione Dati
Questo problema non l'ho capito del tutto e. non l'ho capito bene, ma a quanto pare se si vuole centralizzare, ci sono problemi sui dati. Cioè i dati non possono risiedere fisicamente su altri datacenter che non siano quelli di Wind o Vodafone ad esempio, quindi bisogna creare una tabella per gestire tutti gli utenti in maniera anonima. Banalmente bisogna creare degli identificatori univoci. Un altro problema, è quello di rimandare al datacenter,

