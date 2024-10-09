INSERT INTO persons (first_name, last_name, email) VALUES ('fourth','last','emailfourth');

--// This should fail during testing and transaction should get rolled back. So you should see that the item above never gets inserted.
INSERT INTO personss (first_name, last_name, email) VALUES ('failed','last','faileditemfromtransaction');