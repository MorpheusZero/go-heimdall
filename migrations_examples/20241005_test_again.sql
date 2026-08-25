ALTER TABLE persons ADD COLUMN age INT DEFAULT 0;

INSERT INTO persons (first_name, last_name, email, age) VALUES ('third','last','emailthree', 30);