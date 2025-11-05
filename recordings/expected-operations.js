// Generated from: /tmp/filtered.bin
// MongoDB operations replay script
// Each operation explicitly specifies the database

db.getSiblingDB("traffictest").users.insertMany([
  {
    "_id": "690b9de28e3bbac7703e4bb1",
    "age": 30,
    "city": "New York",
    "name": "Alice",
    "tags": [
      "developer",
      "mongodb"
    ]
  },
  {
    "_id": "690b9de28e3bbac7703e4bb2",
    "age": 25,
    "city": "San Francisco",
    "name": "Bob",
    "tags": [
      "designer",
      "ui"
    ]
  },
  {
    "_id": "690b9de28e3bbac7703e4bb3",
    "age": 35,
    "city": "Boston",
    "name": "Charlie",
    "tags": [
      "manager",
      "agile"
    ]
  }
]);

db.getSiblingDB("traffictest").users.find(
  {
  "age": {
    "$gte": 30
  }
}
).project({
  "_id": 0,
  "age": 1,
  "name": 1
});

db.getSiblingDB("traffictest").users.find(
  {}
).sort({
  "age": -1
});

db.getSiblingDB("traffictest").users.find(
  {}
).sort({
  "age": 1
});

db.getSiblingDB("traffictest").users.find({
  "$or": [
    {
      "age": {
        "$lt": 26
      }
    },
    {
      "city": "Boston"
    }
  ]
});

db.getSiblingDB("traffictest").users.find({
  "$and": [
    {
      "age": {
        "$gte": 25
      }
    },
    {
      "age": {
        "$lte": 35
      }
    }
  ]
});

db.getSiblingDB("traffictest").users.find({
  "tags": {
    "$in": [
      "developer",
      "designer"
    ]
  }
});

db.getSiblingDB("traffictest").users.find({
  "tags": "mongodb"
});

db.getSiblingDB("traffictest").users.find({
  "name": {
    "Pattern": "^A",
    "Options": ""
  }
});

db.getSiblingDB("traffictest").users.updateOne(
  {
  "name": "Alice"
},
  {
  "$inc": {
    "age": 1
  }
}
);

db.getSiblingDB("traffictest").users.updateOne(
  {
  "name": "Bob"
},
  {
  "$mul": {
    "age": 2
  }
}
);

db.getSiblingDB("traffictest").users.updateOne(
  {
  "name": "Alice"
},
  {
  "$min": {
    "age": 28
  }
}
);

db.getSiblingDB("traffictest").users.updateOne(
  {
  "name": "Charlie"
},
  {
  "$max": {
    "age": 40
  }
}
);

db.getSiblingDB("traffictest").users.updateOne(
  {
  "name": "Bob"
},
  {
  "$unset": {
    "city": ""
  }
}
);

db.getSiblingDB("traffictest").users.updateOne(
  {
  "name": "Alice"
},
  {
  "$rename": {
    "tags": "skills"
  }
}
);

db.getSiblingDB("traffictest").users.updateOne(
  {
  "name": "Charlie"
},
  {
  "$addToSet": {
    "tags": "leadership"
  }
}
);

db.getSiblingDB("traffictest").users.updateOne(
  {
  "name": "Charlie"
},
  {
  "$pull": {
    "tags": "agile"
  }
}
);

db.getSiblingDB("traffictest").users.updateOne(
  {
  "name": "Alice"
},
  {
  "$pop": {
    "skills": 1
  }
}
);

db.getSiblingDB("traffictest").users.updateOne(
  {
  "name": "Bob"
},
  {
  "$push": {
    "tags": {
      "$each": [
        "react",
        "vue",
        "angular"
      ],
      "$slice": 5,
      "$sort": 1
    }
  }
}
);

db.getSiblingDB("traffictest").users.findAndModify({
  "new": false,
  "query": {
    "name": "Alice"
  },
  "remove": false,
  "update": {
    "$set": {
      "status": "active"
    }
  },
  "upsert": false
});

db.getSiblingDB("traffictest").users.findAndModify({
  "new": true,
  "query": {
    "name": "Bob"
  },
  "remove": false,
  "update": {
    "$set": {
      "status": "active"
    }
  },
  "upsert": false
});

db.getSiblingDB("traffictest").users.findAndModify({
  "new": false,
  "query": {
    "name": "Charlie"
  },
  "remove": false,
  "update": {
    "age": 36,
    "city": "Boston",
    "name": "Charlie",
    "role": "Senior Manager"
  },
  "upsert": false
});

db.getSiblingDB("traffictest").temp.insertOne({
  "_id": "690b9de28e3bbac7703e4bb4",
  "name": "Temporary",
  "value": 123
});

db.getSiblingDB("traffictest").temp.findAndModify({
  "new": false,
  "query": {
    "name": "Temporary"
  },
  "remove": true,
  "upsert": false
});

db.getSiblingDB("traffictest").users.aggregate([
  {
    "$match": {}
  },
  {
    "$group": {
      "_id": 1,
      "n": {
        "$sum": 1
      }
    }
  }
]);

db.getSiblingDB("traffictest").users.aggregate([
  {
    "$match": {
      "age": {
        "$gte": 30
      }
    }
  },
  {
    "$group": {
      "_id": 1,
      "n": {
        "$sum": 1
      }
    }
  }
]);

db.getSiblingDB("traffictest").runCommand({
  "count": "users"
});

db.getSiblingDB("traffictest").createCollection("logs");

db.getSiblingDB("traffictest").logs.insertOne({
  "_id": "690b9de28e3bbac7703e4bb5",
  "message": "Test log entry",
  "timestamp": "2025-11-05T18:56:34.9Z"
});

db.getSiblingDB("traffictest").temp.insertOne({
  "_id": "690b9de28e3bbac7703e4bb6",
  "test": 1
});

db.getSiblingDB("traffictest").runCommand({
  "authorizedCollections": false,
  "cursor": {},
  "filter": {
    "name": "temp"
  },
  "listCollections": 1,
  "nameOnly": false
});

db.getSiblingDB("traffictest").temp.drop();

db.getSiblingDB("traffictest").runCommand({
  "authorizedCollections": false,
  "cursor": {},
  "filter": {},
  "listCollections": 1,
  "nameOnly": true
});

db.getSiblingDB("traffictest").runCommand({
  "listCollections": 1
});

db.getSiblingDB("traffictest").users.createIndex({
  "name": 1
}, {
  "name": "name_unique",
  "unique": true
});

db.getSiblingDB("traffictest").users.createIndex({
  "age": -1,
  "city": 1
}, {
  "name": "city_age"
});

db.getSiblingDB("traffictest").users.createIndex({
  "status": 1
}, {
  "name": "status_partial"
});

db.getSiblingDB("traffictest").sessions.createIndex({
  "createdAt": 1
}, {
  "name": "session_ttl"
});

db.getSiblingDB("traffictest").runCommand({
  "cursor": {},
  "listIndexes": "users"
});

db.getSiblingDB("traffictest").users.dropIndex("status_partial");

db.getSiblingDB("traffictest").sessions.dropIndex("*");

db.getSiblingDB("traffictest").orders.insertMany([
  {
    "_id": "690b9de38e3bbac7703e4bb7",
    "customerId": 1,
    "date": "2024-01-15T00:00:00Z",
    "item": "laptop",
    "price": 1200,
    "quantity": 1
  },
  {
    "_id": "690b9de38e3bbac7703e4bb8",
    "customerId": 1,
    "date": "2024-01-16T00:00:00Z",
    "item": "mouse",
    "price": 25,
    "quantity": 2
  },
  {
    "_id": "690b9de38e3bbac7703e4bb9",
    "customerId": 2,
    "date": "2024-01-15T00:00:00Z",
    "item": "keyboard",
    "price": 100,
    "quantity": 1
  },
  {
    "_id": "690b9de38e3bbac7703e4bba",
    "customerId": 2,
    "date": "2024-01-20T00:00:00Z",
    "item": "monitor",
    "price": 300,
    "quantity": 2
  },
  {
    "_id": "690b9de38e3bbac7703e4bbb",
    "customerId": 3,
    "date": "2024-02-01T00:00:00Z",
    "item": "laptop",
    "price": 1200,
    "quantity": 1
  }
]);

db.getSiblingDB("traffictest").customers.insertMany([
  {
    "_id": 1,
    "email": "alice@example.com",
    "name": "Alice"
  },
  {
    "_id": 2,
    "email": "bob@example.com",
    "name": "Bob"
  },
  {
    "_id": 3,
    "email": "charlie@example.com",
    "name": "Charlie"
  }
]);

db.getSiblingDB("traffictest").orders.aggregate([
  {
    "$lookup": {
      "as": "customer",
      "foreignField": "_id",
      "from": "customers",
      "localField": "customerId"
    }
  },
  {
    "$limit": 2
  }
]);

db.getSiblingDB("traffictest").orders.aggregate([
  {
    "$bucket": {
      "boundaries": [
        0,
        100,
        500,
        2000
      ],
      "default": "Other",
      "groupBy": "$price",
      "output": {
        "count": {
          "$sum": 1
        },
        "total": {
          "$sum": "$price"
        }
      }
    }
  }
]);

db.getSiblingDB("traffictest").orders.aggregate([
  {
    "$facet": {
      "priceStats": [
        {
          "$group": {
            "_id": null,
            "avgPrice": {
              "$avg": "$price"
            },
            "total": {
              "$sum": 1
            }
          }
        }
      ],
      "topItems": [
        {
          "$group": {
            "_id": "$item",
            "count": {
              "$sum": 1
            }
          }
        },
        {
          "$sort": {
            "count": -1
          }
        },
        {
          "$limit": 3
        }
      ]
    }
  }
]);

db.getSiblingDB("traffictest").orders.aggregate([
  {
    "$sortByCount": "$item"
  }
]);

db.getSiblingDB("traffictest").orders.aggregate([
  {
    "$sample": {
      "size": 2
    }
  }
]);

db.getSiblingDB("traffictest").articles.createIndex({
  "content": "text",
  "title": "text"
}, {
  "name": "text_search"
});

db.getSiblingDB("traffictest").articles.insertMany([
  {
    "_id": "690b9de38e3bbac7703e4bbc",
    "content": "Learn MongoDB fundamentals and CRUD operations",
    "title": "MongoDB Basics"
  },
  {
    "_id": "690b9de38e3bbac7703e4bbd",
    "content": "Master complex aggregation pipelines in MongoDB",
    "title": "Advanced Aggregation"
  },
  {
    "_id": "690b9de38e3bbac7703e4bbe",
    "content": "Introduction to Python programming language",
    "title": "Python Tutorial"
  }
]);

db.getSiblingDB("traffictest").articles.find({
  "$text": {
    "$search": "mongodb aggregation"
  }
});

db.getSiblingDB("traffictest").bulk.insertMany([
  {
    "_id": "690b9de38e3bbac7703e4bbf",
    "name": "Item1",
    "value": 10
  },
  {
    "_id": "690b9de38e3bbac7703e4bc0",
    "name": "Item2",
    "value": 20
  }
]);

db.getSiblingDB("traffictest").bulk.updateOne(
  {
  "name": "Item1"
},
  {
  "$inc": {
    "value": 5
  }
}
);

db.getSiblingDB("traffictest").bulk.deleteOne({
  "name": "Item2"
});

db.getSiblingDB("traffictest").users.aggregate([
  {
    "$collStats": {
      "storageStats": {
        "scale": 1
      }
    }
  }
]);

db.getSiblingDB("traffictest").runCommand({
  "authorizedCollections": false,
  "cursor": {},
  "filter": {},
  "listCollections": 1,
  "nameOnly": true
});


Generated script from 56 packets (56 operations)
