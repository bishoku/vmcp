"""Synthetic, internally consistent banking dataset for the quickstart playground.

Every mock upstream reads from this module, so identifiers line up across services:
a customer's accounts, cards, loans, payments, alerts, cases and complaints all
reference the same IDs. All people, companies, banks and identifiers are fictional.
Standard library only.
"""

BANK_NAME = 'Meridian Bank'
BANK_BIC = 'MRDNGB2L'
AS_OF = '2026-10-28T07:30:00Z'

# --------------------------------------------------------------------------- staff
# Staff references are resolved against demo_catalog users (team, member index).
STAFF_REFS = {
    'rm_private': ('private-banking', 0),
    'rm_sme': ('sme-banking', 0),
    'rm_sme_2': ('sme-banking', 1),
    'rm_corporate': ('corporate-banking', 0),
    'fcc_investigator': ('financial-crime-compliance', 2),
    'fcc_lead': ('financial-crime-compliance', 0),
    'fraud_analyst': ('fraud-strategy', 2),
    'collector': ('collections-recovery', 1),
    'complaints_handler': ('customer-care', 1),
    'complaints_handler_2': ('customer-care', 2),
    'kyc_analyst': ('kyc-operations', 2),
    'disputes_analyst': ('disputes-chargebacks', 1),
    'underwriter': ('lending-origination', 2),
    'trade_officer': ('trade-finance', 1),
    'wealth_advisor': ('wealth-management', 1),
}

# ----------------------------------------------------------------------- branches
BRANCHES = [
    dict(branchId='BR-LON-01', name='London Moorgate', city='London', address='41 Moorgate, London EC2R 6AE', phone='+44 20 7946 0101',
         openingHours='Mon-Fri 09:00-17:00, Sat 09:00-13:00', services=['CASH_DEPOSIT', 'MORTGAGE_ADVICE', 'BUSINESS_BANKING', 'SAFE_DEPOSIT'], accessible=True),
    dict(branchId='BR-LON-02', name='London Canary Wharf Private & Corporate Hub', city='London', address='25 Cabot Square, London E14 4QA', phone='+44 20 7946 0102',
         openingHours='Mon-Fri 08:30-18:00', services=['PRIVATE_BANKING', 'CORPORATE_BANKING', 'TRADE_FINANCE', 'WEALTH_ADVICE'], accessible=True),
    dict(branchId='BR-MAN-01', name='Manchester King Street', city='Manchester', address='62 King Street, Manchester M2 4ND', phone='+44 161 496 0103',
         openingHours='Mon-Fri 09:00-17:00, Sat 09:00-13:00', services=['CASH_DEPOSIT', 'MORTGAGE_ADVICE', 'BUSINESS_BANKING'], accessible=True),
    dict(branchId='BR-BHM-01', name='Birmingham Colmore Row', city='Birmingham', address='118 Colmore Row, Birmingham B3 3AG', phone='+44 121 496 0104',
         openingHours='Mon-Fri 09:00-17:00', services=['CASH_DEPOSIT', 'BUSINESS_BANKING', 'MONEY_GUIDANCE'], accessible=True),
    dict(branchId='BR-EDI-01', name='Edinburgh George Street', city='Edinburgh', address='88 George Street, Edinburgh EH2 3BU', phone='+44 131 496 0105',
         openingHours='Mon-Fri 09:30-16:30', services=['CASH_DEPOSIT', 'MORTGAGE_ADVICE', 'WEALTH_ADVICE'], accessible=True),
    dict(branchId='BR-BRS-01', name='Bristol Corn Street', city='Bristol', address='31 Corn Street, Bristol BS1 1HT', phone='+44 117 496 0106',
         openingHours='Mon-Fri 09:00-17:00, Sat 10:00-13:00', services=['CASH_DEPOSIT', 'BUSINESS_BANKING', 'TRADE_FINANCE'], accessible=False),
]
BRANCH_IDS = [b['branchId'] for b in BRANCHES]

ATMS = [
    dict(atmId='ATM-LON-0101', city='London', location='41 Moorgate (outside branch)', postcode='EC2R 6AE', available24h=True, depositEnabled=True, accessible=True, status='IN_SERVICE'),
    dict(atmId='ATM-LON-0102', city='London', location='Liverpool Street Station concourse', postcode='EC2M 7PY', available24h=True, depositEnabled=False, accessible=True, status='IN_SERVICE'),
    dict(atmId='ATM-LON-0201', city='London', location='25 Cabot Square lobby', postcode='E14 4QA', available24h=False, depositEnabled=True, accessible=True, status='OUT_OF_SERVICE'),
    dict(atmId='ATM-MAN-0101', city='Manchester', location='62 King Street (outside branch)', postcode='M2 4ND', available24h=True, depositEnabled=True, accessible=True, status='IN_SERVICE'),
    dict(atmId='ATM-MAN-0102', city='Manchester', location='Piccadilly Gardens', postcode='M1 1RG', available24h=True, depositEnabled=False, accessible=False, status='IN_SERVICE'),
    dict(atmId='ATM-BHM-0101', city='Birmingham', location='118 Colmore Row (outside branch)', postcode='B3 3AG', available24h=True, depositEnabled=True, accessible=True, status='IN_SERVICE'),
    dict(atmId='ATM-EDI-0101', city='Edinburgh', location='88 George Street (outside branch)', postcode='EH2 3BU', available24h=True, depositEnabled=True, accessible=True, status='IN_SERVICE'),
    dict(atmId='ATM-BRS-0101', city='Bristol', location='31 Corn Street (outside branch)', postcode='BS1 1HT', available24h=True, depositEnabled=True, accessible=False, status='IN_SERVICE'),
    dict(atmId='ATM-BRS-0102', city='Bristol', location='Cabot Circus, Level 1', postcode='BS1 3BX', available24h=False, depositEnabled=False, accessible=True, status='IN_SERVICE'),
]

# ---------------------------------------------------------------------- customers
CUSTOMERS = [
    dict(customerId='CUST-100101', customerType='INDIVIDUAL', fullName='Emma Clarke', segment='PREMIER', dateOfBirth='1986-04-12',
         email='emma.clarke@mail.example', mobile='+44 7700 900101', address='14 Wellington Road, Manchester M14 6BT', city='Manchester',
         customerSince='2012-09-03', status='ACTIVE', riskRating='LOW', kycStatus='VERIFIED', homeBranchId='BR-MAN-01', relationshipManager=None,
         employment='Data Engineering Manager, Northern Analytics Ltd', searchTerms=['emma', 'clarke']),
    dict(customerId='CUST-100102', customerType='INDIVIDUAL', fullName='James Whitaker', segment='MASS_MARKET', dateOfBirth='1991-11-02',
         email='j.whitaker@mail.example', mobile='+44 7700 900102', address='7 Moseley Road, Birmingham B12 0HJ', city='Birmingham',
         customerSince='2017-02-20', status='ACTIVE', riskRating='MEDIUM', kycStatus='VERIFIED', homeBranchId='BR-BHM-01', relationshipManager=None,
         employment='Store Supervisor, Midlands Retail Group', searchTerms=['james', 'whitaker']),
    dict(customerId='CUST-100103', customerType='INDIVIDUAL', fullName='Sophia Patel', segment='PRIVATE_BANKING', dateOfBirth='1974-06-28',
         email='sophia.patel@mail.example', mobile='+44 7700 900103', address='3 Holland Park Avenue, London W11 3RN', city='London',
         customerSince='2009-05-14', status='ACTIVE', riskRating='LOW', kycStatus='VERIFIED', homeBranchId='BR-LON-02', relationshipManager='rm_private',
         employment='Founder and Chief Executive, Patel Health Technologies', searchTerms=['sophia', 'patel']),
    dict(customerId='CUST-100104', customerType='INDIVIDUAL', fullName='Liam Foster', segment='MASS_MARKET', dateOfBirth='1998-01-19',
         email='liam.foster@mail.example', mobile='+44 7700 900104', address='22 Gloucester Road, Bristol BS7 8AE', city='Bristol',
         customerSince='2026-08-02', status='ACTIVE', riskRating='LOW', kycStatus='VERIFIED', homeBranchId='BR-BRS-01', relationshipManager=None,
         employment='Junior Architect, Harbourside Design Studio', searchTerms=['liam', 'foster']),
    dict(customerId='CUST-100105', customerType='INDIVIDUAL', fullName='Olivia Martinez', segment='YOUNG_ADULT', dateOfBirth='2002-09-07',
         email='olivia.martinez@mail.example', mobile='+44 7700 900105', address='Flat 5, 19 Camden Road, London NW1 9LP', city='London',
         customerSince='2020-10-01', status='ACTIVE', riskRating='LOW', kycStatus='VERIFIED', homeBranchId='BR-LON-01', relationshipManager=None,
         employment='UX Designer, Brightline Digital', searchTerms=['olivia', 'martinez']),
    dict(customerId='CUST-100106', customerType='INDIVIDUAL', fullName='Noah Fischer', segment='MASS_MARKET', dateOfBirth='1988-03-30',
         email='noah.fischer@mail.example', mobile='+44 7700 900106', address='10 Marchmont Crescent, Edinburgh EH9 1HN', city='Edinburgh',
         customerSince='2023-04-11', status='ACTIVE', riskRating='MEDIUM', kycStatus='REFRESH_REQUIRED', homeBranchId='BR-EDI-01', relationshipManager=None,
         employment='Research Fellow, University of Edinburgh', searchTerms=['noah', 'fischer']),
    dict(customerId='CUST-100107', customerType='INDIVIDUAL', fullName='Ava Thompson', segment='PREMIER', dateOfBirth='1983-12-15',
         email='ava.thompson@mail.example', mobile='+44 7700 900107', address='56 Clapham Common North Side, London SW4 9RU', city='London',
         customerSince='2014-07-22', status='ACTIVE', riskRating='LOW', kycStatus='VERIFIED', homeBranchId='BR-LON-01', relationshipManager=None,
         employment='Senior Solicitor, Whitmore & Lane LLP', searchTerms=['ava', 'thompson']),
    dict(customerId='CUST-200201', customerType='LIMITED_COMPANY', fullName='Harbor Logistics Ltd', segment='SME', dateOfBirth=None,
         email='finance@harborlogistics.example', mobile='+44 117 496 0201', address='Unit 4, Avonmouth Dock Estate, Bristol BS11 9FG', city='Bristol',
         customerSince='2018-03-05', status='ACTIVE', riskRating='HIGH', kycStatus='ENHANCED_DUE_DILIGENCE', homeBranchId='BR-BRS-01', relationshipManager='rm_sme',
         employment=None, companyNumber='09876543', industry='Freight forwarding and logistics (SIC 52290)', searchTerms=['harbor', 'harbour', 'logistics', '09876543']),
    dict(customerId='CUST-200202', customerType='LIMITED_COMPANY', fullName='Greenfield Organics Ltd', segment='SME', dateOfBirth=None,
         email='accounts@greenfieldorganics.example', mobile='+44 121 496 0202', address='Unit 12, Tyseley Industrial Park, Birmingham B11 2AA', city='Birmingham',
         customerSince='2021-06-17', status='ACTIVE', riskRating='LOW', kycStatus='VERIFIED', homeBranchId='BR-BHM-01', relationshipManager='rm_sme_2',
         employment=None, companyNumber='12345678', industry='Organic food wholesale (SIC 46390)', searchTerms=['greenfield', 'organics', '12345678']),
    dict(customerId='CUST-300301', customerType='PLC', fullName='Atlas Manufacturing PLC', segment='CORPORATE', dateOfBirth=None,
         email='treasury@atlasmanufacturing.example', mobile='+44 20 7946 0301', address='Atlas House, 1 Riverside Way, London SE1 9PG', city='London',
         customerSince='2004-01-12', status='ACTIVE', riskRating='MEDIUM', kycStatus='VERIFIED', homeBranchId='BR-LON-02', relationshipManager='rm_corporate',
         employment=None, companyNumber='01234567', industry='Industrial electronics manufacturing (SIC 26110)', searchTerms=['atlas', 'manufacturing', '01234567']),
]
CUSTOMER = {c['customerId']: c for c in CUSTOMERS}

# ----------------------------------------------------------------------- products
PRODUCTS = [
    dict(productCode='CA-EVERYDAY', name='Everyday Current Account', category='CURRENT_ACCOUNT', monthlyFee=0.0, interestRate=0.0, rateType='NONE',
         overdraftApr=39.9, eligibility='UK residents aged 18+', features=['Contactless Visa Debit card', 'Arranged overdraft up to 2,500 GBP subject to status', 'Faster Payments up to 25,000 GBP per day']),
    dict(productCode='CA-PREMIER', name='Premier Current Account', category='CURRENT_ACCOUNT', monthlyFee=15.0, interestRate=0.0, rateType='NONE',
         overdraftApr=29.9, eligibility='Fee waived with 75,000 GBP savings or investments, or 100,000 GBP annual income paid in', features=['Worldwide family travel insurance', 'Fee-free foreign card spending', 'Priority telephone servicing', 'Arranged overdraft up to 10,000 GBP']),
    dict(productCode='CA-PRIVATE', name='Private Banking Current Account', category='CURRENT_ACCOUNT', monthlyFee=0.0, interestRate=0.0, rateType='NONE',
         overdraftApr=12.9, eligibility='By invitation; minimum 1,000,000 GBP relationship', features=['Dedicated private banker', 'Multi-currency payments', 'Bespoke credit facilities']),
    dict(productCode='CA-YOUNG', name='Young Adult Account', category='CURRENT_ACCOUNT', monthlyFee=0.0, interestRate=1.5, rateType='VARIABLE',
         overdraftApr=0.0, eligibility='Aged 18-25', features=['1.50% AER on balances up to 1,000 GBP', 'Savings pots', 'No overdraft by default']),
    dict(productCode='CA-CURRENCY', name='Currency Account (USD/EUR)', category='CURRENT_ACCOUNT', monthlyFee=5.0, interestRate=0.0, rateType='NONE',
         overdraftApr=0.0, eligibility='Existing Premier or Private Banking customers', features=['Hold and pay in USD or EUR', 'SWIFT payments at preferential FX margin']),
    dict(productCode='SAV-INSTANT', name='Instant Access Saver', category='SAVINGS', monthlyFee=0.0, interestRate=3.75, rateType='VARIABLE',
         overdraftApr=0.0, eligibility='UK residents aged 16+', features=['Unlimited withdrawals', 'Interest paid monthly', 'FSCS protected up to 85,000 GBP']),
    dict(productCode='SAV-REGULAR', name='Regular Saver', category='SAVINGS', monthlyFee=0.0, interestRate=5.10, rateType='FIXED',
         overdraftApr=0.0, eligibility='Current account holders; deposit 25-300 GBP per month', features=['12-month term', 'Penalty-free withdrawals: 3 per year']),
    dict(productCode='SAV-FIXED-1Y', name='1-Year Fixed Rate Bond', category='SAVINGS', monthlyFee=0.0, interestRate=4.35, rateType='FIXED',
         overdraftApr=0.0, eligibility='Minimum deposit 1,000 GBP', features=['Rate fixed for 12 months', 'No withdrawals before maturity']),
    dict(productCode='DC-STANDARD', name='Visa Debit Card', category='CARD', monthlyFee=0.0, interestRate=0.0, rateType='NONE',
         overdraftApr=0.0, eligibility='Issued with current accounts', features=['Contactless up to 100 GBP', 'Apple Pay and Google Pay', 'Freeze and unfreeze in app']),
    dict(productCode='CC-PLATINUM', name='Mastercard Platinum Credit Card', category='CARD', monthlyFee=0.0, interestRate=24.9, rateType='VARIABLE',
         overdraftApr=0.0, eligibility='Aged 18+, income 20,000 GBP+, subject to credit assessment', features=['24.9% APR representative (variable)', '0% on balance transfers for 18 months', 'Up to 56 days interest-free']),
    dict(productCode='CC-INFINITE', name='Visa Infinite Credit Card', category='CARD', monthlyFee=41.25, interestRate=29.9, rateType='VARIABLE',
         overdraftApr=0.0, eligibility='Private Banking customers', features=['Airport lounge access', '1.5 reward points per GBP', 'Concierge service']),
    dict(productCode='PL-STANDARD', name='Personal Loan', category='PERSONAL_LOAN', monthlyFee=0.0, interestRate=9.9, rateType='FIXED',
         overdraftApr=0.0, eligibility='Aged 18+, UK resident, income 15,000 GBP+; 1,000-35,000 GBP over 12-84 months', features=['9.9% APR representative', 'No arrangement fee', 'Early settlement charge capped at 58 days interest']),
    dict(productCode='MTG-FIX5', name='5-Year Fixed Rate Mortgage', category='MORTGAGE', monthlyFee=0.0, interestRate=4.29, rateType='FIXED',
         overdraftApr=0.0, eligibility='Up to 75% loan-to-value; 999 GBP product fee', features=['Rate fixed for 60 months then reverts to 7.24% SVR', 'Overpay up to 10% per year without charge', 'Early repayment charge 5%-1% sliding scale']),
    dict(productCode='MTG-TRACKER', name='Lifetime Tracker Mortgage', category='MORTGAGE', monthlyFee=0.0, interestRate=4.79, rateType='TRACKER',
         overdraftApr=0.0, eligibility='Up to 60% loan-to-value; no product fee', features=['Bank Rate plus 0.79% for the full term', 'No early repayment charges']),
    dict(productCode='BCA-STANDARD', name='Business Current Account', category='BUSINESS', monthlyFee=8.5, interestRate=0.0, rateType='NONE',
         overdraftApr=11.5, eligibility='UK registered businesses with turnover below 6.5m GBP', features=['100 free electronic payments per month', 'Accounting software integration', 'Business debit cards']),
    dict(productCode='BCA-CURRENCY', name='Business Currency Account', category='BUSINESS', monthlyFee=12.0, interestRate=0.0, rateType='NONE',
         overdraftApr=0.0, eligibility='Business Current Account holders', features=['USD and EUR accounts', 'Receive and make SWIFT payments']),
    dict(productCode='BL-TERM', name='Business Term Loan', category='BUSINESS', monthlyFee=0.0, interestRate=7.45, rateType='VARIABLE',
         overdraftApr=0.0, eligibility='Trading 2+ years; 25,000-1,000,000 GBP over 12-120 months', features=['Base rate plus margin from 2.45%', 'Capital repayment holidays available', 'Security may be required']),
    dict(productCode='CORP-TREASURY', name='Corporate Treasury Account', category='CORPORATE', monthlyFee=250.0, interestRate=3.1, rateType='VARIABLE',
         overdraftApr=0.0, eligibility='Corporate clients with turnover above 25m GBP', features=['Multi-currency liquidity pooling', 'Host-to-host and SWIFT connectivity', 'Interest on credit balances at SONIA minus 1.10%']),
    dict(productCode='RCF-CORP', name='Revolving Credit Facility', category='CORPORATE', monthlyFee=0.0, interestRate=5.82, rateType='VARIABLE',
         overdraftApr=0.0, eligibility='Corporate clients subject to credit committee approval', features=['SONIA plus margin from 1.85%', 'Commitment fee 0.65% on undrawn amounts', 'Multi-currency drawdowns']),
]
PRODUCT = {p['productCode']: p for p in PRODUCTS}

# ----------------------------------------------------------------------- accounts
_SORT = {'BR-LON-01': '30-63-11', 'BR-LON-02': '30-63-12', 'BR-MAN-01': '30-63-21', 'BR-BHM-01': '30-63-31', 'BR-EDI-01': '30-63-41', 'BR-BRS-01': '30-63-51'}


def _account(account_id, customer_id, product, name, kind, currency, ledger, holds=0.0, overdraft=0.0, rate=0.0, opened='', restrictions=()):
    branch = CUSTOMER[customer_id]['homeBranchId']
    number = account_id[-8:]
    sort = _SORT[branch]
    return dict(accountId=account_id, customerId=customer_id, holderName=CUSTOMER[customer_id]['fullName'], productCode=product, productName=name,
                accountType=kind, currency=currency, status='ACTIVE', sortCode=sort, accountNumber=number,
                iban='GB82MRDN' + sort.replace('-', '') + number, bic=BANK_BIC, ledgerBalance=round(ledger, 2), holds=round(holds, 2),
                overdraftLimit=round(overdraft, 2), availableBalance=round(ledger - holds + overdraft, 2), interestRate=rate,
                openedOn=opened, branchId=branch, restrictions=list(restrictions))


ACCOUNTS = [
    _account('ACC-10010101', 'CUST-100101', 'CA-PREMIER', 'Premier Current Account', 'CURRENT', 'GBP', 8420.55, 120.00, 2000, 0.0, '2012-09-03'),
    _account('ACC-10010102', 'CUST-100101', 'SAV-INSTANT', 'Instant Access Saver', 'SAVINGS', 'GBP', 25000.00, 0, 0, 3.75, '2015-01-10'),
    _account('ACC-10010201', 'CUST-100102', 'CA-EVERYDAY', 'Everyday Current Account', 'CURRENT', 'GBP', -320.40, 0, 500, 0.0, '2017-02-20'),
    _account('ACC-10010301', 'CUST-100103', 'CA-PRIVATE', 'Private Banking Current Account', 'CURRENT', 'GBP', 54210.00, 0, 25000, 0.0, '2009-05-14'),
    _account('ACC-10010302', 'CUST-100103', 'CA-CURRENCY', 'USD Currency Account', 'CURRENCY', 'USD', 120500.00, 0, 0, 0.0, '2016-08-01'),
    _account('ACC-10010303', 'CUST-100103', 'CA-CURRENCY', 'EUR Currency Account', 'CURRENCY', 'EUR', 13900.00, 0, 0, 0.0, '2019-04-23'),
    _account('ACC-10010401', 'CUST-100104', 'CA-EVERYDAY', 'Everyday Current Account', 'CURRENT', 'GBP', 1250.80, 45.99, 250, 0.0, '2026-08-02'),
    _account('ACC-10010501', 'CUST-100105', 'CA-YOUNG', 'Young Adult Account', 'CURRENT', 'GBP', 2890.10, 0, 0, 1.5, '2020-10-01'),
    _account('ACC-10010502', 'CUST-100105', 'SAV-REGULAR', 'Regular Saver', 'SAVINGS', 'GBP', 6400.00, 0, 0, 5.10, '2025-11-01'),
    _account('ACC-10010601', 'CUST-100106', 'CA-EVERYDAY', 'Everyday Current Account', 'CURRENT', 'GBP', 4120.00, 0, 500, 0.0, '2023-04-11',
             restrictions=['INTERNATIONAL_PAYMENTS_BLOCKED_PENDING_KYC_REFRESH']),
    _account('ACC-10010701', 'CUST-100107', 'CA-PREMIER', 'Premier Current Account', 'CURRENT', 'GBP', 12980.45, 0, 2000, 0.0, '2014-07-22'),
    _account('ACC-20020101', 'CUST-200201', 'BCA-STANDARD', 'Business Current Account', 'BUSINESS_CURRENT', 'GBP', 182340.90, 15000, 50000, 0.0, '2018-03-05'),
    _account('ACC-20020102', 'CUST-200201', 'BCA-CURRENCY', 'Business USD Account', 'BUSINESS_CURRENCY', 'USD', 64200.00, 48500, 0, 0.0, '2019-09-12'),
    _account('ACC-20020201', 'CUST-200202', 'BCA-STANDARD', 'Business Current Account', 'BUSINESS_CURRENT', 'GBP', 46780.25, 0, 20000, 0.0, '2021-06-17'),
    _account('ACC-30030101', 'CUST-300301', 'CORP-TREASURY', 'Corporate Treasury Account GBP', 'CORPORATE', 'GBP', 4820000.00, 0, 0, 3.1, '2004-01-12'),
    _account('ACC-30030102', 'CUST-300301', 'CORP-TREASURY', 'Corporate Treasury Account EUR', 'CORPORATE', 'EUR', 1950000.00, 0, 0, 2.2, '2008-02-01'),
    _account('ACC-30030103', 'CUST-300301', 'CORP-TREASURY', 'Corporate Treasury Account USD', 'CORPORATE', 'USD', 2300000.00, 0, 0, 3.4, '2008-02-01'),
]
ACCOUNT = {a['accountId']: a for a in ACCOUNTS}

# Transactions are listed newest first: (id suffix, booking date, description, counterparty, amount, direction, category, channel, status, reference)
_TX = {
    'ACC-10010101': [
        ('06', '2026-10-27', 'Card purchase', 'Tesco Stores Manchester', 64.18, 'DEBIT', 'GROCERIES', 'CARD', 'BOOKED', 'CRD-5001'),
        ('05', '2026-10-25', 'Salary', 'Northern Analytics Ltd', 4850.00, 'CREDIT', 'SALARY', 'BACS', 'BOOKED', 'OCT26 PAYROLL'),
        ('04', '2026-10-22', 'Faster Payment', 'British Gas', 86.40, 'DEBIT', 'UTILITIES', 'FASTER_PAYMENTS', 'BOOKED', 'PAY-2026102201'),
        ('03', '2026-10-03', 'Credit card repayment', 'Mastercard Platinum CRD-5002', 1200.00, 'DEBIT', 'CARD_REPAYMENT', 'INTERNAL_TRANSFER', 'BOOKED', 'CRD-5002'),
        ('02', '2026-10-01', 'Mortgage payment', 'Meridian Bank Mortgages LN-700101', 1548.20, 'DEBIT', 'MORTGAGE', 'DIRECT_DEBIT', 'BOOKED', 'LN-700101'),
        ('01', '2026-10-01', 'Standing order', 'Daniel Clarke', 150.00, 'DEBIT', 'TRANSFER', 'STANDING_ORDER', 'BOOKED', 'SO-110101'),
    ],
    'ACC-10010102': [
        ('02', '2026-10-01', 'Monthly interest', 'Meridian Bank', 76.85, 'CREDIT', 'INTEREST', 'INTERNAL', 'BOOKED', 'INT OCT26'),
        ('01', '2026-09-12', 'Transfer from current account', 'Emma Clarke ACC-10010101', 2000.00, 'CREDIT', 'TRANSFER', 'INTERNAL_TRANSFER', 'BOOKED', 'SAVINGS'),
    ],
    'ACC-10010201': [
        ('06', '2026-10-24', 'Arranged overdraft interest', 'Meridian Bank', 8.62, 'DEBIT', 'FEES', 'INTERNAL', 'BOOKED', 'OD INT'),
        ('05', '2026-10-22', 'Card purchase', 'Deliveroo', 23.45, 'DEBIT', 'DINING', 'CARD', 'BOOKED', 'CRD-5003'),
        ('04', '2026-10-20', 'Standing order', 'R Hughes (rent)', 750.00, 'DEBIT', 'RENT', 'STANDING_ORDER', 'REJECTED', 'SO-120101 / PAY-2026102003'),
        ('03', '2026-10-15', 'Loan repayment returned unpaid', 'Meridian Bank Loans LN-700102', 317.96, 'DEBIT', 'LOAN_REPAYMENT', 'DIRECT_DEBIT', 'RETURNED', 'LN-700102'),
        ('02', '2026-10-10', 'Salary', 'Midlands Retail Group', 1980.00, 'CREDIT', 'SALARY', 'BACS', 'BOOKED', 'OCT26 PAY'),
        ('01', '2026-10-02', 'Direct debit', 'Birmingham City Council (council tax)', 168.00, 'DEBIT', 'TAX', 'DIRECT_DEBIT', 'BOOKED', 'CTAX 2026'),
    ],
    'ACC-10010301': [
        ('04', '2026-10-26', 'Card purchase', 'Harrods London', 2350.00, 'DEBIT', 'SHOPPING', 'CARD', 'BOOKED', 'CRD-5004'),
        ('03', '2026-10-20', 'Dividend', 'Patel Health Technologies Ltd', 45000.00, 'CREDIT', 'DIVIDEND', 'BACS', 'BOOKED', 'INTERIM DIV 2026'),
        ('02', '2026-10-05', 'Discretionary portfolio fee', 'Meridian Wealth PF-300101', 1605.69, 'DEBIT', 'INVESTMENT_FEES', 'INTERNAL', 'BOOKED', 'PF-300101 Q3'),
        ('01', '2026-10-01', 'Credit card repayment', 'Visa Infinite CRD-5004', 6100.00, 'DEBIT', 'CARD_REPAYMENT', 'INTERNAL_TRANSFER', 'BOOKED', 'CRD-5004'),
    ],
    'ACC-10010302': [
        ('02', '2026-10-14', 'Incoming SWIFT payment', 'Patel Health Technologies Inc (Boston)', 30000.00, 'CREDIT', 'TRANSFER', 'SWIFT', 'BOOKED', 'CONSULTING FEE Q3'),
        ('01', '2026-09-28', 'Outgoing SWIFT payment', 'Cambridge Trust Company', 12000.00, 'DEBIT', 'EDUCATION', 'SWIFT', 'BOOKED', 'TUITION FALL 2026'),
    ],
    'ACC-10010303': [
        ('01', '2026-10-27', 'Outgoing SWIFT payment', 'Villa Serena Property SL', 25000.00, 'DEBIT', 'PROPERTY', 'SWIFT', 'BOOKED', 'UETR a4c2e8f1-3b5d-4e7a-9f10-2c6d8b9e0a03'),
    ],
    'ACC-10010401': [
        ('04', '2026-10-26', 'Card authorisation', 'Amazon Marketplace', 45.99, 'DEBIT', 'SHOPPING', 'CARD', 'PENDING', 'CRD-5005'),
        ('03', '2026-10-24', 'Direct debit', 'Bristol Energy Co-operative', 72.10, 'DEBIT', 'UTILITIES', 'DIRECT_DEBIT', 'BOOKED', 'ENERGY OCT'),
        ('02', '2026-10-23', 'Card purchase', 'Pret A Manger Bristol', 8.35, 'DEBIT', 'DINING', 'CARD', 'BOOKED', 'CRD-5005'),
        ('01', '2026-10-17', 'Salary', 'Harbourside Design Studio', 2150.00, 'CREDIT', 'SALARY', 'BACS', 'BOOKED', 'OCT26 PAY'),
    ],
    'ACC-10010501': [
        ('05', '2026-10-27', 'Card purchase', 'Spotify', 11.99, 'DEBIT', 'SUBSCRIPTIONS', 'CARD', 'BOOKED', 'CRD-5006'),
        ('04', '2026-10-25', 'Card purchase', 'Sainsburys Camden', 42.70, 'DEBIT', 'GROCERIES', 'CARD', 'BOOKED', 'CRD-5006'),
        ('03', '2026-10-24', 'Salary', 'Brightline Digital', 2420.00, 'CREDIT', 'SALARY', 'BACS', 'BOOKED', 'OCT26 PAY'),
        ('02', '2026-10-02', 'Standing order', 'Olivia Martinez Regular Saver', 200.00, 'DEBIT', 'SAVINGS', 'STANDING_ORDER', 'BOOKED', 'SO-150101'),
        ('01', '2026-10-01', 'Rent', 'Camden Lettings Ltd', 1150.00, 'DEBIT', 'RENT', 'FASTER_PAYMENTS', 'BOOKED', 'FLAT 5 OCT'),
    ],
    'ACC-10010502': [
        ('02', '2026-10-02', 'Standing order', 'Olivia Martinez ACC-10010501', 200.00, 'CREDIT', 'SAVINGS', 'STANDING_ORDER', 'BOOKED', 'SO-150101'),
        ('01', '2026-10-01', 'Monthly interest', 'Meridian Bank', 26.31, 'CREDIT', 'INTEREST', 'INTERNAL', 'BOOKED', 'INT OCT26'),
    ],
    'ACC-10010601': [
        ('03', '2026-10-21', 'International payment rejected', 'Fischer Familie (Munich)', 600.00, 'DEBIT', 'TRANSFER', 'SWIFT', 'REJECTED', 'KYC REFRESH REQUIRED'),
        ('02', '2026-10-20', 'Card purchase', 'Waterstones Edinburgh', 23.98, 'DEBIT', 'SHOPPING', 'CARD', 'BOOKED', 'CRD-5007'),
        ('01', '2026-10-15', 'Salary', 'University of Edinburgh', 3240.00, 'CREDIT', 'SALARY', 'BACS', 'BOOKED', 'OCT26 PAY'),
    ],
    'ACC-10010701': [
        ('04', '2026-10-25', 'Salary', 'Whitmore & Lane LLP', 6120.00, 'CREDIT', 'SALARY', 'BACS', 'BOOKED', 'OCT26 PAY'),
        ('03', '2026-10-21', 'Direct debit', 'Thames Water', 48.20, 'DEBIT', 'UTILITIES', 'DIRECT_DEBIT', 'BOOKED', 'TW OCT'),
        ('02', '2026-10-05', 'Credit card repayment', 'Mastercard Platinum CRD-5008', 950.00, 'DEBIT', 'CARD_REPAYMENT', 'INTERNAL_TRANSFER', 'BOOKED', 'CRD-5008'),
        ('01', '2026-10-01', 'Faster Payment', 'Clapham Nursery School', 1340.00, 'DEBIT', 'CHILDCARE', 'FASTER_PAYMENTS', 'BOOKED', 'AUTUMN TERM'),
    ],
    'ACC-20020101': [
        ('06', '2026-10-27', 'CHAPS payment', 'Avonmouth Fuel Supplies Ltd', 12480.00, 'DEBIT', 'SUPPLIERS', 'CHAPS', 'BOOKED', 'PAY-2026102702'),
        ('05', '2026-10-24', 'Incoming payment', 'Severn Retail Distribution plc', 64250.00, 'CREDIT', 'CUSTOMER_RECEIPTS', 'BACS', 'BOOKED', 'INV-4471'),
        ('04', '2026-10-20', 'Loan repayment', 'Meridian Bank Business Loans LN-700201', 3840.00, 'DEBIT', 'LOAN_REPAYMENT', 'DIRECT_DEBIT', 'BOOKED', 'LN-700201'),
        ('03', '2026-10-15', 'Payroll', 'Harbor Logistics staff payroll (42 employees)', 96300.00, 'DEBIT', 'PAYROLL', 'BACS', 'BOOKED', 'PAYROLL OCT'),
        ('02', '2026-10-10', 'Incoming payment', 'Western Freight Brokers Ltd', 118900.00, 'CREDIT', 'CUSTOMER_RECEIPTS', 'FASTER_PAYMENTS', 'BOOKED', 'INV-4459'),
        ('01', '2026-10-01', 'Standing order', 'Bristol Port Company (dock lease)', 8500.00, 'DEBIT', 'RENT', 'STANDING_ORDER', 'BOOKED', 'SO-210101'),
    ],
    'ACC-20020102': [
        ('03', '2026-10-22', 'Outgoing SWIFT payment (on hold)', 'Volkov Trading FZE (Dubai)', 48500.00, 'DEBIT', 'SUPPLIERS', 'SWIFT', 'PENDING', 'UETR 7d1f9a20-4b6e-4f3a-8c2d-91e5b0a4c702'),
        ('02', '2026-10-15', 'Outgoing SWIFT payment', 'Volkov Trading FZE (Dubai)', 46900.00, 'DEBIT', 'SUPPLIERS', 'SWIFT', 'BOOKED', 'FREIGHT CHARTER OCT'),
        ('01', '2026-10-08', 'Incoming SWIFT payment', 'Gulf Maritime Agencies LLC', 72400.00, 'CREDIT', 'CUSTOMER_RECEIPTS', 'SWIFT', 'BOOKED', 'INV-USD-0921'),
    ],
    'ACC-20020201': [
        ('04', '2026-10-26', 'Faster Payment', 'Midlands Packaging Co', 3420.00, 'DEBIT', 'SUPPLIERS', 'FASTER_PAYMENTS', 'BOOKED', 'PO-7781'),
        ('03', '2026-10-21', 'Card settlement', 'Card acquiring settlement (Meridian Merchant Services)', 18650.40, 'CREDIT', 'CARD_SALES', 'BACS', 'BOOKED', 'MMS 21OCT'),
        ('02', '2026-10-14', 'Incoming payment', 'FreshWay Supermarkets Ltd', 27400.00, 'CREDIT', 'CUSTOMER_RECEIPTS', 'BACS', 'BOOKED', 'INV-GO-1022'),
        ('01', '2026-10-05', 'VAT payment', 'HM Revenue & Customs', 9870.00, 'DEBIT', 'TAX', 'FASTER_PAYMENTS', 'BOOKED', 'VAT Q3'),
    ],
    'ACC-30030101': [
        ('03', '2026-10-25', 'Payroll', 'Atlas Manufacturing payroll (1,240 employees)', 3910000.00, 'DEBIT', 'PAYROLL', 'BACS', 'BOOKED', 'PAYROLL OCT'),
        ('02', '2026-10-21', 'RCF drawdown', 'Meridian Bank RCF LN-700301', 1500000.00, 'CREDIT', 'FINANCING', 'INTERNAL', 'BOOKED', 'LN-700301 DRAW 7'),
        ('01', '2026-10-12', 'Incoming CHAPS', 'Northern Rail Systems Ltd', 2750000.00, 'CREDIT', 'CUSTOMER_RECEIPTS', 'CHAPS', 'BOOKED', 'INV-ATL-88120'),
    ],
    'ACC-30030102': [
        ('02', '2026-10-19', 'Incoming SEPA payment', 'Rhein Bau GmbH', 480000.00, 'CREDIT', 'CUSTOMER_RECEIPTS', 'SEPA', 'BOOKED', 'LC-2026-0398 PARTIAL'),
        ('01', '2026-10-06', 'Outgoing SEPA payment', 'Eurocomponents BV', 215000.00, 'DEBIT', 'SUPPLIERS', 'SEPA', 'BOOKED', 'PO-EU-5531'),
    ],
    'ACC-30030103': [
        ('02', '2026-10-23', 'LC settlement', 'Shenzhen Lianhua Electronics Co Ltd', 1250000.00, 'DEBIT', 'TRADE_FINANCE', 'SWIFT', 'BOOKED', 'LC-2026-0412 / UETR e3b0c442-98fc-4c1c-9a3f-6d2b1a7c5e01'),
        ('01', '2026-10-09', 'Incoming SWIFT payment', 'Pacific Grid Solutions Inc', 1820000.00, 'CREDIT', 'CUSTOMER_RECEIPTS', 'SWIFT', 'BOOKED', 'INV-ATL-US-310'),
    ],
}


def account_transactions(account_id):
    acc = ACCOUNT[account_id]
    running = acc['ledgerBalance']
    out = []
    for suffix, day, desc, cp, amount, direction, category, channel, status, ref in _TX.get(account_id, []):
        row = dict(transactionId='TXN-' + account_id[-6:] + '-' + suffix, bookingDate=day, valueDate=day, description=desc, counterparty=cp,
                   amount=amount, currency=acc['currency'], direction=direction, category=category, channel=channel, status=status, reference=ref,
                   runningBalance=None)
        if status == 'BOOKED':
            row['runningBalance'] = round(running, 2)
            running -= amount if direction == 'CREDIT' else -amount
        out.append(row)
    return out


def account_statement(account_id):
    txs = [t for t in account_transactions(account_id) if t['status'] == 'BOOKED']
    credits = round(sum(t['amount'] for t in txs if t['direction'] == 'CREDIT'), 2)
    debits = round(sum(t['amount'] for t in txs if t['direction'] == 'DEBIT'), 2)
    closing = ACCOUNT[account_id]['ledgerBalance']
    return dict(openingBalance=round(closing - credits + debits, 2), closingBalance=closing, totalCredits=credits, totalDebits=debits, transactionCount=len(txs))


# -------------------------------------------------------------------------- cards
CARDS = [
    dict(cardId='CRD-5001', customerId='CUST-100101', cardType='DEBIT', productCode='DC-STANDARD', productName='Visa Debit (Premier)', scheme='VISA', maskedPan='4921 **** **** 4821',
         linkedAccountId='ACC-10010101', status='ACTIVE', expiryDate='2028-07', creditLimit=None, currentBalance=None, availableCredit=None,
         dailyPurchaseLimit=5000.0, dailyAtmLimit=500.0, onlinePaymentsEnabled=True, contactlessEnabled=True, openFraudAlert=False),
    dict(cardId='CRD-5002', customerId='CUST-100101', cardType='CREDIT', productCode='CC-PLATINUM', productName='Mastercard Platinum', scheme='MASTERCARD', maskedPan='5412 **** **** 7710',
         linkedAccountId=None, status='ACTIVE', expiryDate='2027-11', creditLimit=12000.0, currentBalance=1845.20, availableCredit=10154.80,
         dailyPurchaseLimit=6000.0, dailyAtmLimit=300.0, onlinePaymentsEnabled=True, contactlessEnabled=True, openFraudAlert=False),
    dict(cardId='CRD-5003', customerId='CUST-100102', cardType='DEBIT', productCode='DC-STANDARD', productName='Visa Debit', scheme='VISA', maskedPan='4921 **** **** 3398',
         linkedAccountId='ACC-10010201', status='ACTIVE', expiryDate='2027-03', creditLimit=None, currentBalance=None, availableCredit=None,
         dailyPurchaseLimit=2000.0, dailyAtmLimit=300.0, onlinePaymentsEnabled=True, contactlessEnabled=True, openFraudAlert=False),
    dict(cardId='CRD-5004', customerId='CUST-100103', cardType='CREDIT', productCode='CC-INFINITE', productName='Visa Infinite', scheme='VISA', maskedPan='4687 **** **** 0091',
         linkedAccountId=None, status='ACTIVE', expiryDate='2029-01', creditLimit=50000.0, currentBalance=7420.00, availableCredit=42580.00,
         dailyPurchaseLimit=25000.0, dailyAtmLimit=1000.0, onlinePaymentsEnabled=True, contactlessEnabled=True, openFraudAlert=False),
    dict(cardId='CRD-5005', customerId='CUST-100104', cardType='DEBIT', productCode='DC-STANDARD', productName='Visa Debit', scheme='VISA', maskedPan='4921 **** **** 6612',
         linkedAccountId='ACC-10010401', status='FROZEN', expiryDate='2029-08', creditLimit=None, currentBalance=None, availableCredit=None,
         dailyPurchaseLimit=1500.0, dailyAtmLimit=250.0, onlinePaymentsEnabled=True, contactlessEnabled=True, openFraudAlert=False,
         statusReason='Temporarily frozen by customer in mobile app on 2026-10-26 (card misplaced)'),
    dict(cardId='CRD-5006', customerId='CUST-100105', cardType='DEBIT', productCode='DC-STANDARD', productName='Visa Debit', scheme='VISA', maskedPan='4921 **** **** 2087',
         linkedAccountId='ACC-10010501', status='ACTIVE', expiryDate='2028-02', creditLimit=None, currentBalance=None, availableCredit=None,
         dailyPurchaseLimit=1500.0, dailyAtmLimit=250.0, onlinePaymentsEnabled=True, contactlessEnabled=True, openFraudAlert=False),
    dict(cardId='CRD-5007', customerId='CUST-100106', cardType='DEBIT', productCode='DC-STANDARD', productName='Visa Debit', scheme='VISA', maskedPan='4921 **** **** 9154',
         linkedAccountId='ACC-10010601', status='ACTIVE', expiryDate='2027-09', creditLimit=None, currentBalance=None, availableCredit=None,
         dailyPurchaseLimit=2000.0, dailyAtmLimit=300.0, onlinePaymentsEnabled=False, contactlessEnabled=True, openFraudAlert=False),
    dict(cardId='CRD-5008', customerId='CUST-100107', cardType='CREDIT', productCode='CC-PLATINUM', productName='Mastercard Platinum', scheme='MASTERCARD', maskedPan='5412 **** **** 5546',
         linkedAccountId=None, status='ACTIVE', expiryDate='2028-05', creditLimit=15000.0, currentBalance=3230.30, availableCredit=11769.70,
         dailyPurchaseLimit=7500.0, dailyAtmLimit=300.0, onlinePaymentsEnabled=True, contactlessEnabled=True, openFraudAlert=True),
    dict(cardId='CRD-5009', customerId='CUST-200201', cardType='DEBIT', productCode='DC-STANDARD', productName='Visa Business Debit', scheme='VISA', maskedPan='4921 **** **** 1180',
         linkedAccountId='ACC-20020101', status='ACTIVE', expiryDate='2028-10', creditLimit=None, currentBalance=None, availableCredit=None,
         dailyPurchaseLimit=10000.0, dailyAtmLimit=500.0, onlinePaymentsEnabled=True, contactlessEnabled=True, openFraudAlert=False),
]
CARD = {c['cardId']: c for c in CARDS}

# Credit card transactions (debit card transactions are derived from the linked account).
_CARD_TX = {
    'CRD-5002': [
        ('CTX-50020003', '2026-10-24T18:12:00Z', 'John Lewis Manchester', '5311', 'GB', 329.00, 'SETTLED'),
        ('CTX-50020002', '2026-10-19T08:40:00Z', 'Shell Didsbury', '5541', 'GB', 68.40, 'SETTLED'),
        ('CTX-50020004', '2026-09-14T21:04:00Z', 'Hotel Arts Barcelona', '7011', 'ES', 1830.00, 'SETTLED'),
        ('CTX-50020001', '2026-09-02T06:00:00Z', 'FitLife Gyms (membership)', '7997', 'GB', 45.00, 'SETTLED'),
    ],
    'CRD-5004': [
        ('CTX-50040002', '2026-10-21T13:05:00Z', 'The Ritz Paris', '7011', 'FR', 4120.00, 'SETTLED'),
        ('CTX-50040001', '2026-10-11T19:30:00Z', 'British Airways', '3005', 'GB', 3300.00, 'SETTLED'),
    ],
    'CRD-5008': [
        ('CTX-80080005', '2026-10-27T23:44:51Z', 'GLOBALTECH STORE ONLINE', '5732', 'NL', 989.00, 'DECLINED'),
        ('CTX-80080004', '2026-10-27T23:41:07Z', 'GLOBALTECH STORE ONLINE', '5732', 'NL', 1249.99, 'AUTHORISED'),
        ('CTX-80080003', '2026-10-26T17:22:00Z', 'Waitrose Clapham', '5411', 'GB', 84.32, 'SETTLED'),
        ('CTX-80080002', '2026-10-20T07:00:00Z', 'Netflix', '4899', 'GB', 10.99, 'SETTLED'),
        ('CTX-80080001', '2026-10-12T10:15:00Z', 'SkyHigh Airlines', '3299', 'GB', 642.00, 'SETTLED'),
    ],
}


def card_transactions(card_id):
    card = CARD[card_id]
    if card['cardType'] == 'CREDIT':
        rows = []
        for tid, ts, merchant, mcc, country, amount, status in _CARD_TX.get(card_id, []):
            rows.append(dict(transactionId=tid, timestamp=ts, merchantName=merchant, merchantCategoryCode=mcc, merchantCountry=country,
                             amount=amount, currency='GBP', status=status, cardPresent=country == 'GB' and merchant != 'Netflix' and 'ONLINE' not in merchant,
                             disputeReference=DISPUTE_BY_TX.get(tid)))
        return rows
    rows = []
    for t in account_transactions(card['linkedAccountId']):
        if t['channel'] != 'CARD':
            continue
        rows.append(dict(transactionId=t['transactionId'], timestamp=t['bookingDate'] + 'T12:00:00Z', merchantName=t['counterparty'], merchantCategoryCode='5999',
                         merchantCountry='GB', amount=t['amount'], currency=t['currency'], status='SETTLED' if t['status'] == 'BOOKED' else 'AUTHORISED',
                         cardPresent=True, disputeReference=None))
    return rows


# -------------------------------------------------------------------------- loans
def _annuity(principal, annual_rate, months):
    r = annual_rate / 100 / 12
    return round(principal * r / (1 - (1 + r) ** -months), 2)


LOANS = [
    dict(loanId='LN-700101', customerId='CUST-100101', productCode='MTG-FIX5', productName='5-Year Fixed Rate Mortgage', loanType='MORTGAGE',
         originalAmount=285000.00, outstandingPrincipal=241380.12, currency='GBP', interestRate=4.29, rateType='FIXED', fixedUntil='2028-05-31',
         termMonths=300, startDate='2021-06-01', maturityDate='2046-05-31', monthlyPayment=1548.20, nextPaymentDate='2026-11-01',
         status='PERFORMING', daysPastDue=0, arrearsAmount=0.0, collateral='14 Wellington Road, Manchester M14 6BT (valued 415,000 GBP, 2026-04)', loanToValue=58.2,
         repaymentAccountId='ACC-10010101'),
    dict(loanId='LN-700102', customerId='CUST-100102', productCode='PL-STANDARD', productName='Personal Loan', loanType='PERSONAL_LOAN',
         originalAmount=15000.00, outstandingPrincipal=9870.33, currency='GBP', interestRate=9.9, rateType='FIXED', fixedUntil=None,
         termMonths=60, startDate='2024-03-15', maturityDate='2029-03-15', monthlyPayment=317.96, nextPaymentDate='2026-11-15',
         status='DELINQUENT', daysPastDue=47, arrearsAmount=635.92, collateral=None, loanToValue=None, repaymentAccountId='ACC-10010201'),
    dict(loanId='LN-700201', customerId='CUST-200201', productCode='BL-TERM', productName='Business Term Loan', loanType='BUSINESS_TERM_LOAN',
         originalAmount=250000.00, outstandingPrincipal=198400.00, currency='GBP', interestRate=7.45, rateType='VARIABLE', fixedUntil=None,
         termMonths=84, startDate='2024-01-10', maturityDate='2031-01-10', monthlyPayment=3840.00, nextPaymentDate='2026-11-20',
         status='PERFORMING', daysPastDue=0, arrearsAmount=0.0, collateral='Debenture over company assets; director guarantee 100,000 GBP', loanToValue=None,
         repaymentAccountId='ACC-20020101', covenants=['Debt service coverage ratio at least 1.25x (latest 1.41x, 2026-06-30)', 'Annual audited accounts within 180 days of year end']),
    dict(loanId='LN-700301', customerId='CUST-300301', productCode='RCF-CORP', productName='Revolving Credit Facility', loanType='REVOLVING_CREDIT_FACILITY',
         originalAmount=10000000.00, outstandingPrincipal=6250000.00, currency='GBP', interestRate=5.82, rateType='VARIABLE', fixedUntil=None,
         termMonths=60, startDate='2024-07-01', maturityDate='2029-06-30', monthlyPayment=30312.50, nextPaymentDate='2026-11-30',
         status='PERFORMING', daysPastDue=0, arrearsAmount=0.0, collateral='Unsecured; negative pledge and cross-default clauses', loanToValue=None,
         repaymentAccountId='ACC-30030101', covenants=['Net debt to EBITDA at most 2.75x (latest 1.9x, 2026-06-30)', 'Interest cover at least 4.0x (latest 6.2x)']),
]
LOAN = {l['loanId']: l for l in LOANS}


def repayment_schedule(loan_id, count=6):
    loan = LOAN[loan_id]
    balance = loan['outstandingPrincipal']
    rate = loan['interestRate'] / 100 / 12
    year, month, day = map(int, loan['nextPaymentDate'].split('-'))
    out = []
    for n in range(count):
        interest = round(balance * rate, 2)
        principal = 0.0 if loan['loanType'] == 'REVOLVING_CREDIT_FACILITY' else round(loan['monthlyPayment'] - interest, 2)
        payment = round(interest + principal, 2)
        balance = round(balance - principal, 2)
        out.append(dict(installmentNumber=n + 1, dueDate=f'{year:04d}-{month:02d}-{day:02d}', totalPayment=payment, principal=principal,
                        interest=interest, closingBalance=balance, status='DUE' if n else 'NEXT'))
        month += 1
        if month > 12:
            month, year = 1, year + 1
    return out


def early_settlement(loan_id):
    loan = LOAN[loan_id]
    accrued = round(loan['outstandingPrincipal'] * loan['interestRate'] / 100 / 365 * 27, 2)
    charge = {'MORTGAGE': round(loan['outstandingPrincipal'] * 0.03, 2), 'PERSONAL_LOAN': round(loan['outstandingPrincipal'] * loan['interestRate'] / 100 / 365 * 58, 2),
              'BUSINESS_TERM_LOAN': round(loan['outstandingPrincipal'] * 0.01, 2), 'REVOLVING_CREDIT_FACILITY': 0.0}[loan['loanType']]
    basis = {'MORTGAGE': 'Early repayment charge 3% during the fixed-rate period (ends 2028-05-31)', 'PERSONAL_LOAN': 'Statutory maximum of 58 days interest',
             'BUSINESS_TERM_LOAN': 'Break cost 1% of outstanding principal', 'REVOLVING_CREDIT_FACILITY': 'No prepayment fee; drawn amounts may be repaid on any interest date'}[loan['loanType']]
    return dict(loanId=loan_id, currency=loan['currency'], outstandingPrincipal=loan['outstandingPrincipal'], accruedInterest=accrued,
                arrearsAmount=loan['arrearsAmount'], earlyRepaymentCharge=charge, chargeBasis=basis,
                settlementAmount=round(loan['outstandingPrincipal'] + accrued + loan['arrearsAmount'] + charge, 2), validUntil='2026-11-11',
                paymentInstructions='Pay to sort code 30-63-00, account 90000001, reference ' + loan_id)


LOAN_APPLICATIONS = [
    dict(applicationId='APP-880105', customerId='CUST-100105', productCode='PL-STANDARD', requestedAmount=12000.0, termMonths=60, purpose='DEBT_CONSOLIDATION',
         status='UNDER_REVIEW', decision='REFER', submittedAt='2026-10-23T14:02:00Z', underwriter='underwriter',
         outstandingRequirements=['Latest two payslips from Brightline Digital', 'Settlement letter for existing third-party credit card'],
         offer=None, declineReasons=[]),
    dict(applicationId='APP-880202', customerId='CUST-200202', productCode='BL-TERM', requestedAmount=80000.0, termMonths=36, purpose='WORKING_CAPITAL',
         status='APPROVED', decision='APPROVE', submittedAt='2026-10-02T09:30:00Z', underwriter='underwriter', outstandingRequirements=['Signed facility letter', 'Director guarantee 40,000 GBP'],
         offer=dict(amount=80000.0, termMonths=36, interestRate=7.95, monthlyPayment=_annuity(80000, 7.95, 36), arrangementFee=800.0, offerExpiresOn='2026-11-30'),
         declineReasons=[]),
    dict(applicationId='APP-880102', customerId='CUST-100102', productCode='PL-STANDARD', requestedAmount=5000.0, termMonths=36, purpose='OTHER',
         status='DECLINED', decision='DECLINE', submittedAt='2026-08-19T19:45:00Z', underwriter='underwriter', outstandingRequirements=[],
         offer=None, declineReasons=['Missed payments on existing Meridian Bank personal loan LN-700102', 'Affordability below policy threshold']),
]
APPLICATION = {a['applicationId']: a for a in LOAN_APPLICATIONS}

# --------------------------------------------------------------------- credit risk
CREDIT_BUREAU = {
    'CUST-100101': dict(score=921, band='EXCELLENT', activeAccounts=5, missedPayments12m=0, defaults=0, countyCourtJudgments=0, hardSearches6m=0, creditUtilisation=15.4, electoralRoll=True),
    'CUST-100102': dict(score=548, band='POOR', activeAccounts=4, missedPayments12m=3, defaults=0, countyCourtJudgments=0, hardSearches6m=3, creditUtilisation=86.0, electoralRoll=True),
    'CUST-100103': dict(score=965, band='EXCELLENT', activeAccounts=6, missedPayments12m=0, defaults=0, countyCourtJudgments=0, hardSearches6m=0, creditUtilisation=14.8, electoralRoll=True),
    'CUST-100104': dict(score=781, band='FAIR', activeAccounts=1, missedPayments12m=0, defaults=0, countyCourtJudgments=0, hardSearches6m=1, creditUtilisation=0.0, electoralRoll=False),
    'CUST-100105': dict(score=842, band='GOOD', activeAccounts=3, missedPayments12m=0, defaults=0, countyCourtJudgments=0, hardSearches6m=2, creditUtilisation=48.0, electoralRoll=True),
    'CUST-100106': dict(score=803, band='GOOD', activeAccounts=2, missedPayments12m=0, defaults=0, countyCourtJudgments=0, hardSearches6m=0, creditUtilisation=9.0, electoralRoll=True),
    'CUST-100107': dict(score=905, band='EXCELLENT', activeAccounts=4, missedPayments12m=0, defaults=0, countyCourtJudgments=0, hardSearches6m=0, creditUtilisation=21.5, electoralRoll=True),
    'CUST-200201': dict(score=62, band='MODERATE_RISK', activeAccounts=3, missedPayments12m=0, defaults=0, countyCourtJudgments=0, hardSearches6m=1, creditUtilisation=54.0, electoralRoll=None),
    'CUST-200202': dict(score=78, band='LOW_RISK', activeAccounts=2, missedPayments12m=0, defaults=0, countyCourtJudgments=0, hardSearches6m=1, creditUtilisation=22.0, electoralRoll=None),
    'CUST-300301': dict(score=88, band='VERY_LOW_RISK', activeAccounts=7, missedPayments12m=0, defaults=0, countyCourtJudgments=0, hardSearches6m=0, creditUtilisation=62.5, electoralRoll=None),
}

RISK_GRADES = {
    'CUST-100101': dict(grade=2, probabilityOfDefault12m=0.35, monthlyDisposableIncome=2140.0, maxRecommendedUnsecuredExposure=30000.0, policyFlags=[]),
    'CUST-100102': dict(grade=8, probabilityOfDefault12m=14.2, monthlyDisposableIncome=95.0, maxRecommendedUnsecuredExposure=0.0, policyFlags=['ACTIVE_ARREARS', 'FINANCIAL_DIFFICULTY_INDICATOR']),
    'CUST-100103': dict(grade=1, probabilityOfDefault12m=0.12, monthlyDisposableIncome=18500.0, maxRecommendedUnsecuredExposure=250000.0, policyFlags=[]),
    'CUST-100104': dict(grade=5, probabilityOfDefault12m=2.4, monthlyDisposableIncome=640.0, maxRecommendedUnsecuredExposure=5000.0, policyFlags=['THIN_CREDIT_FILE', 'NEW_TO_BANK']),
    'CUST-100105': dict(grade=4, probabilityOfDefault12m=1.6, monthlyDisposableIncome=520.0, maxRecommendedUnsecuredExposure=12000.0, policyFlags=['EXISTING_UNSECURED_DEBT']),
    'CUST-100106': dict(grade=4, probabilityOfDefault12m=1.3, monthlyDisposableIncome=980.0, maxRecommendedUnsecuredExposure=10000.0, policyFlags=['KYC_REFRESH_OUTSTANDING']),
    'CUST-100107': dict(grade=2, probabilityOfDefault12m=0.41, monthlyDisposableIncome=2870.0, maxRecommendedUnsecuredExposure=35000.0, policyFlags=[]),
    'CUST-200201': dict(grade=6, probabilityOfDefault12m=3.9, monthlyDisposableIncome=21000.0, maxRecommendedUnsecuredExposure=150000.0, policyFlags=['OPEN_AML_INVESTIGATION', 'HIGH_RISK_CORRIDOR_EXPOSURE']),
    'CUST-200202': dict(grade=3, probabilityOfDefault12m=0.9, monthlyDisposableIncome=7400.0, maxRecommendedUnsecuredExposure=120000.0, policyFlags=[]),
    'CUST-300301': dict(grade=3, probabilityOfDefault12m=0.6, monthlyDisposableIncome=None, maxRecommendedUnsecuredExposure=15000000.0, policyFlags=['CREDIT_COMMITTEE_APPROVAL_REQUIRED']),
}

# ------------------------------------------------------------------ collections
COLLECTION_CASES = [
    dict(caseId='COL-900101', customerId='CUST-100102', loanId='LN-700102', accountId='ACC-10010201', stage='EARLY_ARREARS', daysPastDue=47,
         arrearsAmount=635.92, missedPayments=2, assignedCollector='collector', openedOn='2026-09-25', status='OPEN',
         vulnerabilityIndicators=['FINANCIAL_DIFFICULTY'], lastContact=dict(date='2026-10-20', channel='SMS', outcome='Reminder delivered; no response'),
         promiseToPay=None, recommendedTreatment='Offer a 6-month reduced payment arrangement; signpost free debt advice (MoneyHelper)'),
]

# -------------------------------------------------------------- fraud and AML
FRAUD_ALERTS = [
    dict(alertId='FRA-400101', customerId='CUST-100107', cardId='CRD-5008', transactionId='CTX-80080004', createdAt='2026-10-27T23:42:10Z', riskScore=0.94,
         severity='HIGH', status='OPEN', rule='Card-not-present purchase at a new merchant in a new country, followed by a declined second attempt within 5 minutes',
         amount=1249.99, currency='GBP', assignedAnalyst='fraud_analyst', customerContacted=True,
         recommendedAction='Verify with the customer. If unrecognised: confirm fraud, freeze card CRD-5008 and open a chargeback for CTX-80080004.'),
    dict(alertId='FRA-400102', customerId='CUST-100101', cardId='CRD-5002', transactionId='CTX-50020004', createdAt='2026-09-14T21:05:00Z', riskScore=0.71,
         severity='MEDIUM', status='CLOSED_GENUINE', rule='High-value hotel payment abroad without travel notification', amount=1830.00, currency='GBP',
         assignedAnalyst='fraud_analyst', customerContacted=True, recommendedAction='None - customer confirmed genuine travel spend'),
]
RISK_SCORES = {
    'CTX-80080004': dict(score=0.94, decision='REVIEW', reasons=['New merchant for this card', 'Merchant country NL differs from cardholder pattern', 'Transaction at 23:41 outside usual activity hours', 'Amount 6.8x the 90-day average']),
    'CTX-80080005': dict(score=0.97, decision='DECLINE', reasons=['Second attempt at the same merchant within 4 minutes', 'Velocity rule V-12 triggered', 'Device fingerprint not previously seen']),
    'TXN-020102-03': dict(score=0.62, decision='REVIEW', reasons=['Newly added beneficiary in high-risk corridor', 'Round-number freight payments above 45,000 USD']),
    'TXN-020102-02': dict(score=0.58, decision='REVIEW', reasons=['First payment to beneficiary', 'Beneficiary bank located in high-risk corridor']),
}

AML_CASES = [
    dict(caseId='AML-700101', customerId='CUST-200201', alertType='HIGH_RISK_CORRIDOR_WIRES', status='UNDER_INVESTIGATION', priority='HIGH',
         openedOn='2026-10-23', dueDate='2026-11-06', assignedInvestigator='fcc_investigator',
         summary='Two outbound USD wires (46,900 and 48,500 USD) to newly added beneficiary Volkov Trading FZE, Dubai. Beneficiary name is a partial match (0.87) to a sanctions list entry. The second wire is held.',
         linkedTransactions=['TXN-020102-02', 'TXN-020102-03'], linkedPayments=['7d1f9a20-4b6e-4f3a-8c2d-91e5b0a4c702'],
         screeningHits=['VOLKOV TRADING FZE - UK HMT consolidated list (simulated), match score 0.87'], sarDecision='PENDING', riskRating='HIGH',
         nextSteps=['Obtain charter contract and bills of lading from customer', 'Confirm beneficiary ultimate beneficial owner', 'Decide on SAR submission by 2026-11-06']),
    dict(caseId='AML-700102', customerId='CUST-300301', alertType='LARGE_INBOUND_CROSS_BORDER', status='CLOSED_NO_FURTHER_ACTION', priority='LOW',
         openedOn='2026-10-20', dueDate='2026-10-27', assignedInvestigator='fcc_lead',
         summary='Inbound SEPA receipt of 480,000 EUR from Rhein Bau GmbH. Matched to export letter of credit LC-2026-0398; consistent with customer profile.',
         linkedTransactions=['TXN-030102-02'], linkedPayments=[], screeningHits=[], sarDecision='NOT_REQUIRED', riskRating='LOW',
         nextSteps=[]),
]

KYC = {
    'CUST-100101': dict(status='VERIFIED', lastReviewDate='2024-03-01', nextReviewDue='2027-03-01', documents=[('PASSPORT', 'VERIFIED', '2031-05-20'), ('PROOF_OF_ADDRESS', 'VERIFIED', None)], outstanding=[], openCaseId=None),
    'CUST-100102': dict(status='VERIFIED', lastReviewDate='2023-11-10', nextReviewDue='2026-11-10', documents=[('DRIVING_LICENCE', 'VERIFIED', '2030-11-02'), ('PROOF_OF_ADDRESS', 'VERIFIED', None)], outstanding=['Periodic review due 2026-11-10'], openCaseId=None),
    'CUST-100103': dict(status='VERIFIED', lastReviewDate='2026-02-15', nextReviewDue='2027-02-15', documents=[('PASSPORT', 'VERIFIED', '2033-01-09'), ('SOURCE_OF_WEALTH', 'VERIFIED', None), ('PROOF_OF_ADDRESS', 'VERIFIED', None)], outstanding=[], openCaseId=None),
    'CUST-100104': dict(status='VERIFIED', lastReviewDate='2026-08-02', nextReviewDue='2029-08-02', documents=[('PASSPORT', 'VERIFIED', '2034-02-11'), ('PROOF_OF_ADDRESS', 'VERIFIED', None)], outstanding=[], openCaseId=None),
    'CUST-100105': dict(status='VERIFIED', lastReviewDate='2025-10-01', nextReviewDue='2028-10-01', documents=[('PASSPORT', 'VERIFIED', '2030-07-30'), ('PROOF_OF_ADDRESS', 'VERIFIED', None)], outstanding=[], openCaseId=None),
    'CUST-100106': dict(status='REFRESH_REQUIRED', lastReviewDate='2023-04-11', nextReviewDue='2026-09-30', documents=[('NATIONAL_ID_CARD', 'VERIFIED', '2029-12-31'), ('PROOF_OF_ADDRESS', 'EXPIRED', '2026-09-30'), ('VISA_OR_RESIDENCE_PERMIT', 'VERIFIED', '2028-04-10')], outstanding=['Proof of address dated within the last 3 months', 'Updated employment confirmation'], openCaseId='KYC-550106'),
    'CUST-100107': dict(status='VERIFIED', lastReviewDate='2025-06-30', nextReviewDue='2028-06-30', documents=[('PASSPORT', 'VERIFIED', '2032-09-18'), ('PROOF_OF_ADDRESS', 'VERIFIED', None)], outstanding=[], openCaseId=None),
    'CUST-200201': dict(status='ENHANCED_DUE_DILIGENCE', lastReviewDate='2025-03-05', nextReviewDue='2026-11-15', documents=[('CERTIFICATE_OF_INCORPORATION', 'VERIFIED', None), ('UBO_REGISTER', 'PENDING_VERIFICATION', None), ('DIRECTOR_ID', 'VERIFIED', '2030-06-01'), ('SOURCE_OF_FUNDS', 'REQUESTED', None)], outstanding=['Ultimate beneficial owner confirmation for new 30% shareholder', 'Source of funds evidence for Dubai freight contracts'], openCaseId='KYC-550201'),
    'CUST-200202': dict(status='VERIFIED', lastReviewDate='2025-06-17', nextReviewDue='2027-06-17', documents=[('CERTIFICATE_OF_INCORPORATION', 'VERIFIED', None), ('UBO_REGISTER', 'VERIFIED', None), ('DIRECTOR_ID', 'VERIFIED', '2031-03-12')], outstanding=[], openCaseId=None),
    'CUST-300301': dict(status='VERIFIED', lastReviewDate='2026-01-12', nextReviewDue='2027-01-12', documents=[('CERTIFICATE_OF_INCORPORATION', 'VERIFIED', None), ('LISTED_ENTITY_CONFIRMATION', 'VERIFIED', None), ('AUTHORISED_SIGNATORIES', 'VERIFIED', None)], outstanding=[], openCaseId=None),
}

KYC_DOCUMENTS = {
    'INDIVIDUAL': ['Valid passport, UK photocard driving licence or national ID card', 'Proof of address dated within 3 months (utility bill, council tax or bank statement)'],
    'SOLE_TRADER': ['Proprietor photo ID', 'Proof of personal address dated within 3 months', 'Evidence of trading (invoices or HMRC self-assessment)'],
    'LIMITED_COMPANY': ['Certificate of incorporation', 'Register of persons with significant control (UBOs above 25%)', 'Photo ID and proof of address for all directors and UBOs', 'Latest filed accounts or management accounts'],
    'PLC': ['Confirmation of listing on a regulated market', 'Register of directors and authorised signatories', 'Board resolution authorising the account mandate'],
}
KYC_EDD_DOCUMENTS = ['Source of funds and source of wealth evidence', 'Description of expected transaction corridors and counterparties', 'Senior management approval record', 'Adverse media screening report']

# ---------------------------------------------------------------- payments data
PAYEES = [
    dict(payeeId='PAYEE-1101', customerId='CUST-100101', name='British Gas', sortCode='40-11-62', accountNumber='21876543', reference='CUST 8810021', addedOn='2016-02-03', closeMatch='british'),
    dict(payeeId='PAYEE-1102', customerId='CUST-100101', name='Daniel Clarke', sortCode='20-45-77', accountNumber='63829104', reference='MONTHLY', addedOn='2019-06-11', closeMatch='clarke'),
    dict(payeeId='PAYEE-1201', customerId='CUST-100102', name='R Hughes', sortCode='09-01-28', accountNumber='44120987', reference='RENT 7 MOSELEY RD', addedOn='2022-09-01', closeMatch='hughes'),
    dict(payeeId='PAYEE-1501', customerId='CUST-100105', name='Camden Lettings Ltd', sortCode='60-40-05', accountNumber='70033412', reference='FLAT 5', addedOn='2024-08-20', closeMatch='camden'),
    dict(payeeId='PAYEE-1701', customerId='CUST-100107', name='Clapham Nursery School', sortCode='30-99-50', accountNumber='11902233', reference='AVA THOMPSON CHILD', addedOn='2025-08-28', closeMatch='clapham'),
    dict(payeeId='PAYEE-2101', customerId='CUST-200201', name='Avonmouth Fuel Supplies Ltd', sortCode='40-52-40', accountNumber='81726354', reference='HARBOR LOG', addedOn='2018-04-15', closeMatch='avonmouth'),
    dict(payeeId='PAYEE-2201', customerId='CUST-200202', name='Midlands Packaging Co', sortCode='20-98-61', accountNumber='55667788', reference='GREENFIELD', addedOn='2021-07-02', closeMatch='midlands'),
]

PAYMENTS = [
    dict(paymentId='PAY-2026102201', sourceAccountId='ACC-10010101', beneficiaryName='British Gas', amount=86.40, currency='GBP', scheme='FASTER_PAYMENTS',
         status='COMPLETED', createdAt='2026-10-22T09:14:00Z', settledAt='2026-10-22T09:14:06Z', rejectionReason=None, reference='CUST 8810021'),
    dict(paymentId='PAY-2026102702', sourceAccountId='ACC-20020101', beneficiaryName='Avonmouth Fuel Supplies Ltd', amount=12480.00, currency='GBP', scheme='CHAPS',
         status='COMPLETED', createdAt='2026-10-27T10:02:00Z', settledAt='2026-10-27T10:31:00Z', rejectionReason=None, reference='HARBOR LOG'),
    dict(paymentId='PAY-2026102003', sourceAccountId='ACC-10010201', beneficiaryName='R Hughes', amount=750.00, currency='GBP', scheme='FASTER_PAYMENTS',
         status='REJECTED', createdAt='2026-10-20T06:00:00Z', settledAt=None, rejectionReason='INSUFFICIENT_FUNDS', reference='RENT 7 MOSELEY RD'),
]

STANDING_ORDERS = [
    dict(standingOrderId='SO-110101', accountId='ACC-10010101', payeeName='Daniel Clarke', amount=150.00, frequency='MONTHLY', nextPaymentDate='2026-11-01', status='ACTIVE', lastPaymentStatus='PAID', reference='MONTHLY'),
    dict(standingOrderId='SO-110102', accountId='ACC-10010101', payeeName='Manchester Rowing Club', amount=25.00, frequency='MONTHLY', nextPaymentDate='2026-11-05', status='ACTIVE', lastPaymentStatus='PAID', reference='MEMBERSHIP EC'),
    dict(standingOrderId='SO-120101', accountId='ACC-10010201', payeeName='R Hughes', amount=750.00, frequency='MONTHLY', nextPaymentDate='2026-11-20', status='ACTIVE', lastPaymentStatus='REJECTED_INSUFFICIENT_FUNDS', reference='RENT 7 MOSELEY RD'),
    dict(standingOrderId='SO-150101', accountId='ACC-10010501', payeeName='Olivia Martinez Regular Saver', amount=200.00, frequency='MONTHLY', nextPaymentDate='2026-11-02', status='ACTIVE', lastPaymentStatus='PAID', reference='SAVINGS'),
    dict(standingOrderId='SO-210101', accountId='ACC-20020101', payeeName='Bristol Port Company', amount=8500.00, frequency='MONTHLY', nextPaymentDate='2026-11-01', status='ACTIVE', lastPaymentStatus='PAID', reference='DOCK LEASE U4'),
]

SWIFT_PAYMENTS = [
    dict(uetr='e3b0c442-98fc-4c1c-9a3f-6d2b1a7c5e01', paymentId='IPY-2026102301', customerId='CUST-300301', debtorAccountId='ACC-30030103',
         beneficiaryName='Shenzhen Lianhua Electronics Co Ltd', beneficiaryBic='PRCBCNSZ', amount=1250000.00, currency='USD', status='ACCC',
         statusDescription='Credited to beneficiary account', initiatedAt='2026-10-23T08:05:00Z', completedAt='2026-10-23T14:47:00Z', totalFees=45.00,
         hops=[('MRDNGB2L', 'Meridian Bank London', '2026-10-23T08:06:00Z', 'ACSP'), ('HDSNUS33', 'Hudson Global Bank New York', '2026-10-23T13:10:00Z', 'ACSP'),
               ('PRCBCNSZ', 'Pearl River Commercial Bank Shenzhen', '2026-10-23T14:47:00Z', 'ACCC')]),
    dict(uetr='7d1f9a20-4b6e-4f3a-8c2d-91e5b0a4c702', paymentId='IPY-2026102202', customerId='CUST-200201', debtorAccountId='ACC-20020102',
         beneficiaryName='Volkov Trading FZE', beneficiaryBic='GDXBAEAD', amount=48500.00, currency='USD', status='ON_HOLD',
         statusDescription='Held at originating bank for financial crime compliance review (case AML-700101)', initiatedAt='2026-10-22T11:20:00Z', completedAt=None, totalFees=0.0,
         hops=[('MRDNGB2L', 'Meridian Bank London', '2026-10-22T11:21:00Z', 'PDNG')]),
    dict(uetr='a4c2e8f1-3b5d-4e7a-9f10-2c6d8b9e0a03', paymentId='IPY-2026102703', customerId='CUST-100103', debtorAccountId='ACC-10010303',
         beneficiaryName='Villa Serena Property SL', beneficiaryBic='COSTESMM', amount=25000.00, currency='EUR', status='ACSP',
         statusDescription='In progress at intermediary bank', initiatedAt='2026-10-27T15:40:00Z', completedAt=None, totalFees=12.50,
         hops=[('MRDNGB2L', 'Meridian Bank London', '2026-10-27T15:41:00Z', 'ACSP'), ('EUCLDEFF', 'Euroclear Settlement Bank Frankfurt', '2026-10-28T07:05:00Z', 'ACSP')]),
]

# --------------------------------------------------------------------------- FX
_GBP_MID = {'GBP': 1.0, 'USD': 1.2734, 'EUR': 1.1652, 'CHF': 1.1218, 'JPY': 193.42, 'AED': 4.6766}
FX_CURRENCIES = list(_GBP_MID)


def fx_mid(base, quote):
    return round(_GBP_MID[quote] / _GBP_MID[base], 6 if _GBP_MID[quote] / _GBP_MID[base] < 10 else 4)


def fx_table(base):
    rows = []
    for quote in FX_CURRENCIES:
        if quote == base:
            continue
        mid = fx_mid(base, quote)
        digits = 4 if mid < 10 else 2
        rows.append(dict(currency=quote, mid=mid, bankBuys=round(mid * 1.006, digits), bankSells=round(mid * 0.994, digits),
                         change24hPercent=_CHANGE.get(base + quote, 0.0)))
    return rows


_CHANGE = {'GBPUSD': 0.18, 'GBPEUR': -0.07, 'GBPCHF': 0.04, 'GBPJPY': 0.32, 'GBPAED': 0.18, 'USDGBP': -0.18, 'USDEUR': -0.25, 'USDCHF': -0.14,
           'USDJPY': 0.14, 'USDAED': 0.0, 'EURGBP': 0.07, 'EURUSD': 0.25, 'EURCHF': 0.11, 'EURJPY': 0.39, 'EURAED': 0.25}
FX_QUOTE_PAIRS = [('GBP', 'USD'), ('GBP', 'EUR'), ('GBP', 'AED'), ('GBP', 'CHF'), ('GBP', 'JPY'), ('USD', 'GBP'), ('EUR', 'GBP'),
                  ('USD', 'EUR'), ('EUR', 'USD'), ('USD', 'AED')]
CUSTOMER_FX_MARGIN = 0.004

RATE_CURVES = {
    'SONIA': [('O/N', 4.70), ('1M', 4.68), ('3M', 4.61), ('6M', 4.48), ('1Y', 4.25), ('2Y', 3.98), ('5Y', 3.81), ('10Y', 3.92)],
    'EURIBOR': [('O/N', 3.15), ('1M', 3.18), ('3M', 3.24), ('6M', 3.20), ('1Y', 3.05), ('2Y', 2.71), ('5Y', 2.58), ('10Y', 2.74)],
    'SOFR': [('O/N', 5.31), ('1M', 5.32), ('3M', 5.27), ('6M', 5.12), ('1Y', 4.86), ('2Y', 4.42), ('5Y', 4.05), ('10Y', 4.11)],
}

LIQUIDITY = dict(asOf=AS_OF, liquidityCoverageRatio=148.2, lcrRegulatoryMinimum=100.0, lcrInternalTrigger=120.0, netStableFundingRatio=131.5,
                 highQualityLiquidAssets=18420000000.0, netCashOutflows30d=12430000000.0, reportingCurrency='GBP', status='WITHIN_APPETITE',
                 positions=[dict(currency='GBP', cashAtCentralBank=9850000000.0, netPosition30d=2310000000.0, limitUtilisationPercent=41.0),
                            dict(currency='EUR', cashAtCentralBank=2140000000.0, netPosition30d=-180000000.0, limitUtilisationPercent=63.0),
                            dict(currency='USD', cashAtCentralBank=3020000000.0, netPosition30d=420000000.0, limitUtilisationPercent=38.0)],
                 alerts=['EUR 30-day net position within 10% of internal limit; Treasury ALCO notified 2026-10-27'])

# ------------------------------------------------------------- trade finance
LETTERS_OF_CREDIT = [
    dict(lcReference='LC-2026-0412', customerId='CUST-300301', direction='IMPORT', type='IRREVOCABLE_DOCUMENTARY', status='SETTLED',
         applicant='Atlas Manufacturing PLC', beneficiary='Shenzhen Lianhua Electronics Co Ltd', issuingBank='Meridian Bank London (MRDNGB2L)',
         advisingBank='Pearl River Commercial Bank Shenzhen (PRCBCNSZ)', amount=1250000.00, currency='USD', issueDate='2026-08-04', expiryDate='2026-11-30',
         latestShipmentDate='2026-10-05', incoterm='FOB Shenzhen', portOfLoading='Yantian, Shenzhen', portOfDischarge='Felixstowe',
         goods='12,000 industrial power control modules (HS 8537.10)', documentsRequired=['Commercial invoice', 'Full set clean on-board bills of lading', 'Packing list', 'Certificate of origin', 'Insurance certificate 110%'],
         discrepancies=[], utilisedAmount=1250000.00, settlementUetr='e3b0c442-98fc-4c1c-9a3f-6d2b1a7c5e01'),
    dict(lcReference='LC-2026-0398', customerId='CUST-300301', direction='EXPORT', type='CONFIRMED_DOCUMENTARY', status='DOCUMENTS_PRESENTED_WITH_DISCREPANCIES',
         applicant='Rhein Bau GmbH', beneficiary='Atlas Manufacturing PLC', issuingBank='Rheinische Handelsbank Dusseldorf (RHBKDEDD)',
         advisingBank='Meridian Bank London (MRDNGB2L) - confirming bank', amount=960000.00, currency='EUR', issueDate='2026-07-15', expiryDate='2026-12-15',
         latestShipmentDate='2026-11-20', incoterm='CIP Dusseldorf', portOfLoading='Tilbury', portOfDischarge='Rotterdam',
         goods='Rail signalling control cabinets, partial shipments allowed', documentsRequired=['Commercial invoice', 'CMR or bill of lading', 'Inspection certificate', 'Certificate of origin'],
         discrepancies=['Inspection certificate dated after shipment date', 'Invoice description differs from LC goods description (second shipment)'], utilisedAmount=480000.00, settlementUetr=None),
    dict(lcReference='LC-2026-0455', customerId='CUST-200201', direction='IMPORT', type='IRREVOCABLE_DOCUMENTARY', status='ISSUED',
         applicant='Harbor Logistics Ltd', beneficiary='Nordic Container Leasing AS', issuingBank='Meridian Bank London (MRDNGB2L)',
         advisingBank='Fjord Merchant Bank Oslo (FJMBNOKK)', amount=310000.00, currency='USD', issueDate='2026-10-09', expiryDate='2027-01-31',
         latestShipmentDate='2026-12-15', incoterm='DAP Avonmouth', portOfLoading='Oslo', portOfDischarge='Avonmouth',
         goods='40 refurbished 40ft reefer containers', documentsRequired=['Commercial invoice', 'Bill of lading', 'Inspection certificate'],
         discrepancies=[], utilisedAmount=0.0, settlementUetr=None),
]
BANK_GUARANTEES = [
    dict(guaranteeId='BG-2026-0077', customerId='CUST-300301', type='PERFORMANCE_BOND', beneficiary='Rhein Bau GmbH', amount=250000.00, currency='EUR',
         issueDate='2026-07-20', expiryDate='2027-12-31', status='ACTIVE', underlyingContract='Rail signalling supply contract RB-2026-114', claimsMade=0),
    dict(guaranteeId='BG-2026-0102', customerId='CUST-200202', type='RENT_GUARANTEE', beneficiary='Tyseley Industrial Estates Ltd', amount=36000.00, currency='GBP',
         issueDate='2026-03-01', expiryDate='2027-02-28', status='ACTIVE', underlyingContract='Lease of Unit 12, Tyseley Industrial Park', claimsMade=0),
    dict(guaranteeId='BG-2025-0310', customerId='CUST-200201', type='CUSTOMS_GUARANTEE', beneficiary='HM Revenue & Customs', amount=150000.00, currency='GBP',
         issueDate='2025-05-01', expiryDate='2026-12-31', status='ACTIVE', underlyingContract='Customs deferment account guarantee', claimsMade=0),
]

# ------------------------------------------------------------------- wealth
PORTFOLIOS = [
    dict(portfolioId='PF-300101', customerId='CUST-100103', name='Balanced Growth Discretionary Mandate', mandateType='DISCRETIONARY', currency='GBP',
         marketValue=1284550.00, ytdReturnPercent=6.8, oneYearReturnPercent=9.4, riskLevel=4, inceptionDate='2015-03-01', manager='wealth_advisor',
         assetAllocation=dict(equities=52.0, fixedIncome=31.0, alternatives=9.0, cash=8.0),
         holdings=[('GB00MRD0EQ01', 'Meridian Global Equity Index Fund', 'EQUITY_FUND', 0.30), ('GB00MRD0UK02', 'Meridian UK Equity Income Fund', 'EQUITY_FUND', 0.12),
                   ('US00EMRG0003', 'Emerging Markets Equity ETF', 'ETF', 0.10), ('GB00GILT2032', 'UK Treasury Gilt 4.25% 2032', 'GOVERNMENT_BOND', 0.18),
                   ('XS00CORP0005', 'Sterling Investment Grade Corporate Bond Fund', 'BOND_FUND', 0.13), ('GB00INFRA006', 'Global Infrastructure Trust plc', 'INVESTMENT_TRUST', 0.09),
                   ('CASH-GBP', 'GBP cash', 'CASH', 0.08)]),
    dict(portfolioId='PF-300102', customerId='CUST-100101', name='Stocks and Shares ISA', mandateType='ADVISORY', currency='GBP',
         marketValue=42300.75, ytdReturnPercent=8.9, oneYearReturnPercent=12.1, riskLevel=5, inceptionDate='2018-04-06', manager='wealth_advisor',
         assetAllocation=dict(equities=85.0, fixedIncome=10.0, alternatives=0.0, cash=5.0),
         holdings=[('GB00MRD0EQ01', 'Meridian Global Equity Index Fund', 'EQUITY_FUND', 0.60), ('US00TECH0007', 'Global Technology Leaders ETF', 'ETF', 0.25),
                   ('GB00GILT2032', 'UK Treasury Gilt 4.25% 2032', 'GOVERNMENT_BOND', 0.10), ('CASH-GBP', 'GBP cash', 'CASH', 0.05)]),
]


def portfolio_holdings(portfolio_id):
    p = next(x for x in PORTFOLIOS if x['portfolioId'] == portfolio_id)
    rows, total = [], 0.0
    for i, (isin, name, kind, weight) in enumerate(p['holdings']):
        value = round(p['marketValue'] * weight, 2) if i < len(p['holdings']) - 1 else round(p['marketValue'] - total, 2)
        total += value
        rows.append(dict(instrumentId=isin, name=name, assetClass=kind, weightPercent=round(weight * 100, 2), marketValue=value, currency=p['currency']))
    return rows


RISK_PROFILES = {
    'CUST-100103': dict(profile='BALANCED', riskLevel=4, scale='1 (cautious) to 7 (speculative)', assessedOn='2026-03-10', nextReviewDue='2027-03-10',
                        capacityForLoss='MEDIUM', knowledgeAndExperience='EXTENSIVE', investmentHorizonYears=10,
                        objectives=['Long-term capital growth', 'Sustainable income from 2030'], restrictions=['No tobacco or controversial weapons exposure']),
    'CUST-100101': dict(profile='GROWTH', riskLevel=5, scale='1 (cautious) to 7 (speculative)', assessedOn='2025-04-02', nextReviewDue='2026-11-02',
                        capacityForLoss='MEDIUM', knowledgeAndExperience='INFORMED', investmentHorizonYears=20,
                        objectives=['Retirement savings', 'Use annual ISA allowance'], restrictions=[]),
}

# --------------------------------------------------------- customer servicing
CONTACT_PREFERENCES = {
    'CUST-100101': dict(email=True, sms=True, push=True, post=False, marketingOptIn=True, preferredChannel='PUSH', vulnerabilityIndicators=[]),
    'CUST-100102': dict(email=True, sms=True, push=False, post=True, marketingOptIn=False, preferredChannel='SMS', vulnerabilityIndicators=['FINANCIAL_DIFFICULTY']),
    'CUST-100103': dict(email=True, sms=False, push=False, post=True, marketingOptIn=False, preferredChannel='SECURE_MESSAGE', vulnerabilityIndicators=[]),
    'CUST-100104': dict(email=True, sms=True, push=True, post=False, marketingOptIn=True, preferredChannel='PUSH', vulnerabilityIndicators=[]),
    'CUST-100105': dict(email=True, sms=True, push=True, post=False, marketingOptIn=True, preferredChannel='PUSH', vulnerabilityIndicators=[]),
    'CUST-100106': dict(email=True, sms=False, push=True, post=True, marketingOptIn=False, preferredChannel='EMAIL', vulnerabilityIndicators=[]),
    'CUST-100107': dict(email=True, sms=True, push=True, post=False, marketingOptIn=False, preferredChannel='SMS', vulnerabilityIndicators=[]),
    'CUST-200201': dict(email=True, sms=True, push=False, post=True, marketingOptIn=False, preferredChannel='EMAIL', vulnerabilityIndicators=[]),
    'CUST-200202': dict(email=True, sms=True, push=True, post=False, marketingOptIn=True, preferredChannel='EMAIL', vulnerabilityIndicators=[]),
    'CUST-300301': dict(email=True, sms=False, push=False, post=True, marketingOptIn=False, preferredChannel='SECURE_MESSAGE', vulnerabilityIndicators=[]),
}

NOTIFICATION_TEMPLATES = [
    dict(templateId='FRAUD_ALERT_VERIFY', channels=['SMS', 'PUSH'], purpose='Ask the customer to confirm or reject a suspicious transaction', parameters=['merchantName', 'amount']),
    dict(templateId='CARD_FROZEN', channels=['SMS', 'PUSH', 'EMAIL'], purpose='Confirm a card has been frozen', parameters=['cardLast4']),
    dict(templateId='PAYMENT_CONFIRMATION', channels=['PUSH', 'EMAIL'], purpose='Confirm an outgoing payment', parameters=['amount', 'beneficiaryName']),
    dict(templateId='KYC_DOCUMENT_REQUEST', channels=['EMAIL', 'SECURE_MESSAGE', 'POST'], purpose='Request updated identity or address documents', parameters=['documentList', 'dueDate']),
    dict(templateId='ARREARS_REMINDER', channels=['SMS', 'EMAIL', 'POST'], purpose='Remind a customer about missed payments and offer support', parameters=['arrearsAmount', 'loanId']),
    dict(templateId='APPOINTMENT_CONFIRMATION', channels=['SMS', 'EMAIL'], purpose='Confirm a branch or video appointment', parameters=['branchName', 'date', 'timeSlot']),
    dict(templateId='COMPLAINT_ACKNOWLEDGEMENT', channels=['EMAIL', 'POST', 'SECURE_MESSAGE'], purpose='Acknowledge a complaint and share the reference', parameters=['complaintId']),
    dict(templateId='DISPUTE_UPDATE', channels=['EMAIL', 'PUSH', 'SECURE_MESSAGE'], purpose='Update the customer on a card dispute', parameters=['disputeId', 'status']),
]

NOTIFICATIONS = {
    'CUST-100107': [dict(messageId='MSG-77010001', templateId='FRAUD_ALERT_VERIFY', channel='SMS', sentAt='2026-10-27T23:42:30Z', status='DELIVERED', customerResponse='NO_RESPONSE'),
                    dict(messageId='MSG-77010000', templateId='DISPUTE_UPDATE', channel='EMAIL', sentAt='2026-10-19T10:00:00Z', status='OPENED', customerResponse=None)],
    'CUST-100102': [dict(messageId='MSG-77020002', templateId='ARREARS_REMINDER', channel='SMS', sentAt='2026-10-20T10:00:00Z', status='DELIVERED', customerResponse='NO_RESPONSE'),
                    dict(messageId='MSG-77020001', templateId='ARREARS_REMINDER', channel='EMAIL', sentAt='2026-10-05T09:00:00Z', status='OPENED', customerResponse=None)],
    'CUST-100106': [dict(messageId='MSG-77030001', templateId='KYC_DOCUMENT_REQUEST', channel='EMAIL', sentAt='2026-10-01T08:30:00Z', status='OPENED', customerResponse=None)],
    'CUST-100104': [dict(messageId='MSG-77040001', templateId='CARD_FROZEN', channel='PUSH', sentAt='2026-10-26T20:11:00Z', status='DELIVERED', customerResponse=None)],
}

DISPUTES = [
    dict(disputeId='DSP-600101', customerId='CUST-100107', cardId='CRD-5008', transactionId='CTX-80080001', merchantName='SkyHigh Airlines', amount=642.00, currency='GBP',
         reasonCode='SERVICE_NOT_PROVIDED', status='CHARGEBACK_SUBMITTED', openedOn='2026-10-14', provisionalCreditIssued=True, provisionalCreditAmount=642.00,
         schemeDeadline='2026-12-10', assignedAnalyst='disputes_analyst', timeline=['2026-10-14 Dispute opened by customer (flight cancelled, no refund)',
                                                                                     '2026-10-15 Provisional credit 642.00 GBP applied', '2026-10-21 Chargeback submitted to Mastercard (reason 4855)', '2026-10-28 Awaiting merchant response']),
    dict(disputeId='DSP-600102', customerId='CUST-100101', cardId='CRD-5002', transactionId='CTX-50020001', merchantName='FitLife Gyms (membership)', amount=45.00, currency='GBP',
         reasonCode='CANCELLED_RECURRING', status='RESOLVED_IN_CUSTOMER_FAVOUR', openedOn='2026-09-05', provisionalCreditIssued=True, provisionalCreditAmount=45.00,
         schemeDeadline='2026-10-31', assignedAnalyst='disputes_analyst', timeline=['2026-09-05 Dispute opened', '2026-09-20 Merchant accepted chargeback', '2026-09-21 Credit made permanent']),
]
DISPUTE_BY_TX = {d['transactionId']: d['disputeId'] for d in DISPUTES}

COMPLAINTS = [
    dict(complaintId='CMP-200101', customerId='CUST-100107', category='CARD_DISPUTE', channel='PHONE', status='INVESTIGATING', openedOn='2026-10-18',
         summary='Customer unhappy with time taken to resolve SkyHigh Airlines dispute DSP-600101', relatedReference='DSP-600101', handler='complaints_handler',
         acknowledgedOn='2026-10-18', finalResponseDue='2026-12-13', outcome=None, redressPaid=0.0),
    dict(complaintId='CMP-200102', customerId='CUST-100102', category='FEES_AND_CHARGES', channel='BRANCH', status='RESOLVED', openedOn='2026-09-22',
         summary='Customer disputes overdraft charges applied while in financial difficulty', relatedReference='ACC-10010201', handler='complaints_handler_2',
         acknowledgedOn='2026-09-22', finalResponseDue='2026-11-17', outcome='PARTIALLY_UPHELD - 25.00 GBP charges refunded; referred to collections support', redressPaid=25.00),
    dict(complaintId='CMP-200103', customerId='CUST-200201', category='PAYMENT_DELAY', channel='RELATIONSHIP_MANAGER', status='OPEN', openedOn='2026-10-24',
         summary='International payment to Volkov Trading FZE held since 2026-10-22; customer requests release', relatedReference='7d1f9a20-4b6e-4f3a-8c2d-91e5b0a4c702', handler='complaints_handler',
         acknowledgedOn='2026-10-24', finalResponseDue='2026-12-19', outcome=None, redressPaid=0.0),
]


def by_customer(rows, customer_id):
    return [r for r in rows if r.get('customerId') == customer_id]


def staff_label(key, users_by_ref):
    """Resolve a STAFF_REFS key to a public staff reference using catalog users."""
    if key is None:
        return None
    user = users_by_ref[key]
    return dict(username=user['username'], name=user['firstName'] + ' ' + user['lastName'], team=user['team'], title=user['title'])
