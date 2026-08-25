INSERT INTO persons (first_name, last_name, email, age) VALUES ('fourth','last','emailfourth', 40);

--// This should fail during testing and transaction should get rolled back. So you should see that the item above never gets inserted.
INSERT INTO personss (first_name, last_name, email, age) VALUES ('failed','last','faileditemfromtransaction', 50);