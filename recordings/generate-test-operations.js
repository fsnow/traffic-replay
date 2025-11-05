// Comprehensive test operations for traffic-replay
// Run this against a MongoDB instance while recording traffic
// Usage: mongosh mongodb://localhost:27017/traffictest < generate-test-operations.js

// Clean slate
db.dropDatabase();

print("\n=== 1. BASIC INSERTS (already tested) ===");
db.users.insertMany([
  { name: "Alice", age: 30, city: "New York", tags: ["developer", "mongodb"] },
  { name: "Bob", age: 25, city: "San Francisco", tags: ["designer", "ui"] },
  { name: "Charlie", age: 35, city: "Boston", tags: ["manager", "agile"] }
]);

print("\n=== 2. FIND WITH OPTIONS ===");

// Find with projection
db.users.find({ age: { $gte: 30 } }, { name: 1, age: 1, _id: 0 });

// Find with sort
db.users.find({}).sort({ age: -1 });

// Find with skip and limit
db.users.find({}).sort({ age: 1 }).skip(1).limit(1);

// Find with complex queries
db.users.find({ $or: [{ age: { $lt: 26 } }, { city: "Boston" }] });
db.users.find({ $and: [{ age: { $gte: 25 } }, { age: { $lte: 35 } }] });

// Find with array queries
db.users.find({ tags: { $in: ["developer", "designer"] } });
db.users.find({ tags: "mongodb" });

// Find with regex
db.users.find({ name: /^A/ });

print("\n=== 3. UPDATE OPERATORS ===");

// Numeric operators
db.users.updateOne({ name: "Alice" }, { $inc: { age: 1 } });
db.users.updateOne({ name: "Bob" }, { $mul: { age: 2 } });
db.users.updateOne({ name: "Alice" }, { $min: { age: 28 } });
db.users.updateOne({ name: "Charlie" }, { $max: { age: 40 } });

// Field operators
db.users.updateOne({ name: "Bob" }, { $unset: { city: "" } });
db.users.updateOne({ name: "Alice" }, { $rename: { tags: "skills" } });

// Array operators
db.users.updateOne({ name: "Charlie" }, { $addToSet: { tags: "leadership" } });
db.users.updateOne({ name: "Charlie" }, { $pull: { tags: "agile" } });
db.users.updateOne({ name: "Alice" }, { $pop: { skills: 1 } }); // Remove last element

// Array with modifiers
db.users.updateOne(
  { name: "Bob" },
  {
    $push: {
      tags: {
        $each: ["react", "vue", "angular"],
        $sort: 1,
        $slice: 5
      }
    }
  }
);

print("\n=== 4. FIND AND MODIFY OPERATIONS ===");

// findOneAndUpdate (returns original document)
db.users.findOneAndUpdate(
  { name: "Alice" },
  { $set: { status: "active" } },
  { returnDocument: "before" }
);

// findOneAndUpdate (returns new document)
db.users.findOneAndUpdate(
  { name: "Bob" },
  { $set: { status: "active" } },
  { returnDocument: "after" }
);

// findOneAndReplace
db.users.findOneAndReplace(
  { name: "Charlie" },
  { name: "Charlie", age: 36, city: "Boston", role: "Senior Manager" }
);

// findOneAndDelete
db.temp.insertOne({ name: "Temporary", value: 123 });
db.temp.findOneAndDelete({ name: "Temporary" });

print("\n=== 5. COUNT OPERATIONS ===");

db.users.countDocuments({});
db.users.countDocuments({ age: { $gte: 30 } });
db.users.estimatedDocumentCount();

print("\n=== 6. COLLECTION OPERATIONS ===");

// Create collection with options
db.createCollection("logs", { capped: true, size: 100000, max: 100 });
db.logs.insertOne({ message: "Test log entry", timestamp: new Date() });

// Drop collection
db.temp.insertOne({ test: 1 });
db.temp.drop();

// List collections
db.getCollectionNames();
db.runCommand({ listCollections: 1 });

print("\n=== 7. INDEX OPERATIONS ===");

// Unique index
db.users.createIndex({ name: 1 }, { unique: true, name: "name_unique" });

// Compound index
db.users.createIndex({ city: 1, age: -1 }, { name: "city_age" });

// Partial index
db.users.createIndex(
  { status: 1 },
  { partialFilterExpression: { age: { $gte: 30 } }, name: "status_partial" }
);

// TTL index
db.sessions.createIndex({ createdAt: 1 }, { expireAfterSeconds: 3600, name: "session_ttl" });

// List indexes
db.users.getIndexes();

// Drop specific index
db.users.dropIndex("status_partial");

// Drop all indexes except _id
db.sessions.dropIndexes();

print("\n=== 8. ADVANCED AGGREGATION ===");

// Setup data for aggregation
db.orders.insertMany([
  { customerId: 1, item: "laptop", price: 1200, quantity: 1, date: new Date("2024-01-15") },
  { customerId: 1, item: "mouse", price: 25, quantity: 2, date: new Date("2024-01-16") },
  { customerId: 2, item: "keyboard", price: 100, quantity: 1, date: new Date("2024-01-15") },
  { customerId: 2, item: "monitor", price: 300, quantity: 2, date: new Date("2024-01-20") },
  { customerId: 3, item: "laptop", price: 1200, quantity: 1, date: new Date("2024-02-01") }
]);

db.customers.insertMany([
  { _id: 1, name: "Alice", email: "alice@example.com" },
  { _id: 2, name: "Bob", email: "bob@example.com" },
  { _id: 3, name: "Charlie", email: "charlie@example.com" }
]);

// $lookup (join)
db.orders.aggregate([
  {
    $lookup: {
      from: "customers",
      localField: "customerId",
      foreignField: "_id",
      as: "customer"
    }
  },
  { $limit: 2 }
]);

// $bucket
db.orders.aggregate([
  {
    $bucket: {
      groupBy: "$price",
      boundaries: [0, 100, 500, 2000],
      default: "Other",
      output: { count: { $sum: 1 }, total: { $sum: "$price" } }
    }
  }
]);

// $facet (multiple pipelines)
db.orders.aggregate([
  {
    $facet: {
      priceStats: [
        { $group: { _id: null, avgPrice: { $avg: "$price" }, total: { $sum: 1 } } }
      ],
      topItems: [
        { $group: { _id: "$item", count: { $sum: 1 } } },
        { $sort: { count: -1 } },
        { $limit: 3 }
      ]
    }
  }
]);

// $sortByCount
db.orders.aggregate([
  { $sortByCount: "$item" }
]);

// $sample
db.orders.aggregate([
  { $sample: { size: 2 } }
]);

print("\n=== 9. TEXT SEARCH ===");

// Create text index
db.articles.createIndex({ content: "text", title: "text" }, { name: "text_search" });

db.articles.insertMany([
  { title: "MongoDB Basics", content: "Learn MongoDB fundamentals and CRUD operations" },
  { title: "Advanced Aggregation", content: "Master complex aggregation pipelines in MongoDB" },
  { title: "Python Tutorial", content: "Introduction to Python programming language" }
]);

// Text search
db.articles.find({ $text: { $search: "mongodb aggregation" } });

print("\n=== 10. BULK WRITE ===");

db.bulk.bulkWrite([
  { insertOne: { document: { name: "Item1", value: 10 } } },
  { insertOne: { document: { name: "Item2", value: 20 } } },
  { updateOne: { filter: { name: "Item1" }, update: { $inc: { value: 5 } } } },
  { deleteOne: { filter: { name: "Item2" } } }
]);

print("\n=== 11. ADMIN OPERATIONS ===");

// List databases
db.adminCommand({ listDatabases: 1 });

// Collection stats
db.users.stats();

// Database stats
db.stats();

print("\n=== COMPLETE ===");
print("All test operations executed successfully!");
print("Total collections created: " + db.getCollectionNames().length);
