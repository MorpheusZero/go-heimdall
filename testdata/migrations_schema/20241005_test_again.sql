ALTER TABLE persons_schema ADD COLUMN age INT DEFAULT 0;

INSERT INTO persons_schema (first_name, last_name, email, age) VALUES ('third','last','emailthree', 30);
