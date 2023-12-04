# HW2 Q3

I uses a PostgreSQL default database. Tables are created by

```sql
CREATE TABLE likes (person INT, artist INT);
CREATE TABLE friend (person1 INT, person2 INT);
```

Then create indexes

```sql
CREATE INDEX idx_like_person ON likes (person);
CREATE INDEX idx_like_artist ON likes (artist);
CREATE INDEX idx_friend_person1 ON friend (person1);
CREATE INDEX idx_friend_person2 ON friend (person2);
```

Then I load the sample data into the tables from the txt files.

Then I use this query

```sql
SELECT DISTINCT mutal_friends.p1 AS person, mutal_friends.p2 AS friend, dlikes.artist AS artist
FROM (SELECT person1 AS p1, person2 AS p2 FROM friend UNION SELECT person2 as p1, person1 as p2 FROM friend) AS mutal_friends
INNER JOIN (SELECT DISTINCT * FROM likes) AS dlikes ON mutal_friends.p2 = dlikes.person
WHERE NOT EXISTS(SELECT * from likes AS likes2 WHERE likes2.person = mutal_friends.p1 AND likes2.artist = dlikes.artist);
```

In total, including importing data, those finished in about 30 seconds on my computer with Intel(R) Xeon(R) CPU D-1548 @ 2.00GHz, which is about the same power of normal laptops, or even worse, as the frequency is only 2GHz. And counting the query above states that there are `7308105` lines of tuples fetched.