db = db.getSiblingDB('essaim');
printjson(db.agents.findOne({_id: 'bdd4ca41-735b-4151-8580-566861001000'}));
