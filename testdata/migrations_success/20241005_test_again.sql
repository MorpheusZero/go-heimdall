ALTER TABLE persons_success ADD COLUMN age INT DEFAULT 0;

INSERT INTO persons_success (first_name, last_name, email, age) VALUES ('third','last','emailthree', 30);
