# BROUILLON : L'ANCIENNE VISION VS LA NOUVELLE VISION (Le Postier et le Résumeur)

Ce document compile et résume les idées transmises par messages vocaux concernant le véritable flux de communication au sein d'ESSAIM-IA. Il corrige l'ancienne conception d'une "hiérarchie strictement verticale" qui s'avérait trop lourde et non scalable.

## 1. LE POSTIER (OU ROUTEUR) : LE CŒUR DE LA BULLE
L'idée que les exécutants (les Workers) s'adressent directement à leur Architecte est révolue. C'est inefficace et cela surchargerait ce dernier.
Désormais, les agents d'un même niveau (qui travaillent fondamentalement ensemble sur le même sujet) s'organisent en **Bulle circulaire** autour d'une entité centrale : le **Postier**.

- **Le Rôle du Postier :** Il se positionne au milieu de tous les agents exécutants. Ces agents ne parlent qu'à lui.
- **Routage de l'Information :** Le Postier récupère continuellement les productions. Il décide *intelligemment* s'il garde l'information ou s'il doit la redistribuer à un autre agent du groupe pour l'aider dans sa tâche.
- **Exemple :** Si les agents 1, 2, 3, 4 et 5 produisent des éléments indispensables à l'agent 6, le Postier les intercepte, en fait une synthèse (via un Résumeur) et l'envoie à l'agent 6 pour éviter que celui-ci ne lise "4 pages de texte" et n'explose son contexte.

## 2. LES RÉSUMEURS : LA CLÉ DE LA SCALABILITÉ
L'information brute qui s'accumule coûte cher (en tokens et en attention). Le système intègre donc des **Résumeurs** : des agents ou fonctions intermédiaires spécialisées dans la digestion de la donnée.
- **Le Résumeur du Groupe :** Il aide le Postier à condenser les informations avant de les rediriger (ex: résumer la production de 5 agents pour un seul destinataire).
- **Le Résumeur de Sortie (ou de Niveau Supérieur) :** L'Architecte possède un Résumeur d'entrée et de sortie. Lorsque la bulle d'agents a produit suffisamment de blocs (par exemple 10 ou 12 productions), le Résumeur compile tout le travail de l'étage pour en faire un condensé global.
- L'Architecte lit uniquement ce condensé pour vérifier si le système tourne bien. Si c'est le cas, il laisse tourner sans intervenir.

## 3. RÈGLES DE TAILLE ET DE CRÉATION DE GROUPE
L'Architecte décide du nombre d'agents nécessaires et le système déploie automatiquement l'infrastructure (Postiers/Résumeurs) de support adaptée. 
L'idéal constant est d'avoir des groupes de **3 à 5 exécutants maximum** pour un fonctionnement optimal.

Voici les règles dynamiques de peuplement de la bulle :

- **Si 2 agents :** Le groupe est trop petit. Pas de Postier (ce qui signifie souvent une mauvaise délégation à la base).
- **De 3 à 4 agents :** 1 Postier au centre. (Pas de Résumeur nécessaire).
- **De 5 à 8 agents :** 1 Postier + 1 Résumeur.
- **De 8 à 12 agents :** 1 Postier + 2 Résumeurs.
- **Limite Matérielle (Max 12) :** Il ne faut théoriquement jamais dépasser 12 agents exécutants dans un groupe. Au-delà, l'Architecte doit redécouper la tâche en sous-niveaux. 
- *Exception :* Le système n'est pas idiot, s'il faut générer un "système solaire de 25 éléments inséparables", il le fera sans découper artificiellement, mais c'est une situation extrême qui sera coûteuse et difficile à gérer pour son Résumeur.

## SYNTHÈSE GLOBALE
- Le réseau n'est plus un arbre vertical rigide. 
- C'est un ensemble de **bulles d'exécutants** au sein desquelles se trouve un **Postier**.
- Les **Résumeurs** sont omniprésents entre les bulles et au-dessus d'elles pour que les données massives deviennent des condensés scalables pour les Architectes et les agents isolés.
