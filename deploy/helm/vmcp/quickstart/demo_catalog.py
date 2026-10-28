"""Organisation catalog for the banking playground: teams, SSO users, projects,
virtual servers, runtime agent keys. Standard library only; all names fictional.

The installer, the Keycloak seeding and demo.sh all read this module, so team
membership shown by demo.sh matches the users and groups created in Keycloak.
"""

EMAIL_DOMAIN = 'meridianbank.example'
ADMIN_GROUP = 'vmcp-platform-admins'
VIEWER_GROUP = 'vmcp-auditors'

ROLE_TITLES = {
    'platform': ['Engineering Manager', 'Product Owner', 'Senior Software Engineer', 'Site Reliability Engineer'],
    'business': ['Head of Team', 'Product Manager', 'Senior Business Analyst', 'Operations Specialist'],
    'risk': ['Head of Team', 'Senior Analyst', 'Investigator', 'Quality Assurance Analyst'],
    'ai': ['AI Engineering Lead', 'Product Manager', 'Machine Learning Engineer', 'Conversation Designer'],
}

# slug, display name, kind, mission statement
TEAMS = [
    ('core-banking-platform', 'Core Banking Platform', 'platform', 'Runs the core ledger, deposit accounts, balances and statements.'),
    ('customer-data-management', 'Customer Data Management', 'platform', 'Owns the customer master (CIF), contact data and consent records.'),
    ('payments-engineering', 'Domestic Payments Engineering', 'platform', 'Builds Faster Payments, BACS, CHAPS, payees and standing orders.'),
    ('cross-border-payments', 'Cross-Border Payments', 'platform', 'Operates SWIFT gpi payments, FX quoting for payments and tracking.'),
    ('cards-issuing', 'Card Issuing & Processing', 'platform', 'Issues debit and credit cards and manages card controls.'),
    ('disputes-chargebacks', 'Disputes & Chargebacks', 'business', 'Handles card disputes, provisional credits and scheme chargebacks.'),
    ('fraud-strategy', 'Fraud Strategy & Detection', 'risk', 'Runs real-time fraud scoring, alerting and investigation.'),
    ('financial-crime-compliance', 'Financial Crime Compliance', 'risk', 'AML transaction monitoring, sanctions/PEP screening and SAR decisions.'),
    ('kyc-operations', 'KYC Operations', 'risk', 'Customer due diligence, periodic reviews and document verification.'),
    ('credit-risk-analytics', 'Credit Risk Analytics', 'risk', 'Credit bureau integration, internal risk grades and affordability.'),
    ('lending-origination', 'Lending Origination', 'business', 'Personal, mortgage and business loan quotes, applications and decisions.'),
    ('lending-servicing', 'Loan Servicing', 'business', 'Live loan administration, repayment schedules and settlements.'),
    ('collections-recovery', 'Collections & Recovery', 'business', 'Arrears management, forbearance and payment arrangements.'),
    ('treasury-markets', 'Treasury & Markets', 'business', 'FX and interest rate market data, liquidity and ALM.'),
    ('trade-finance', 'Trade Finance', 'business', 'Letters of credit, guarantees and documentary collections.'),
    ('wealth-management', 'Wealth Management', 'business', 'Investment portfolios, suitability and advice.'),
    ('product-pricing', 'Product & Pricing', 'business', 'Product catalogue, terms, interest rates and fees.'),
    ('branch-network', 'Branch Network & ATM Channels', 'business', 'Branches, ATMs and in-person appointments.'),
    ('digital-engagement', 'Digital Engagement', 'platform', 'Customer notifications across SMS, email, push and secure messages.'),
    ('customer-care', 'Customer Care Operations', 'business', 'Complaint handling and resolution within regulatory deadlines.'),
    ('retail-digital-banking', 'Retail Digital Banking', 'ai', 'Mobile app experiences and the retail banking assistant.'),
    ('sme-banking', 'SME Banking', 'business', 'Relationship management for small and medium-sized businesses.'),
    ('corporate-banking', 'Corporate & Commercial Banking', 'business', 'Large corporate relationships, cash management and trade.'),
    ('private-banking', 'Private Banking', 'business', 'High-net-worth relationships and bespoke banking.'),
    ('contact-center-ai', 'Contact Center AI', 'ai', 'Agent assist, IVR voice banking and conversational automation.'),
    ('operational-risk', 'Operational Risk Management', 'risk', 'Risk and control monitoring across operations.'),
    ('regulatory-reporting', 'Regulatory Reporting', 'risk', 'Prudential and conduct regulatory returns.'),
    ('internal-audit', 'Internal Audit', 'risk', 'Independent assurance and audit evidence collection.'),
    ('data-science-ai-lab', 'AI & Data Science Lab', 'ai', 'Experimentation, next-best-action models and the GenAI sandbox.'),
    ('open-banking-apis', 'Open Banking APIs', 'platform', 'Regulated account information and payment initiation APIs.'),
]

_FIRST = '''Oliver Charlotte Henry Grace Arthur Freya Thomas Lily William Evelyn Jack Alice Samuel Poppy Daniel Florence Joseph Matilda
Benjamin Ivy Lucas Rosie Edward Phoebe Alexander Ella Leo Harriet Oscar Imogen Theodore Eleanor Max Scarlett Isaac Hannah Jacob Sienna
Ethan Zara Adam Maya Ryan Chloe Nathan Ruby Aaron Layla Luke Megan Elliot Abigail Caleb Naomi Gabriel Victoria Dylan Bethany Owen Jasmine
Toby Leah Finley Clara Hugo Esme Felix Lucy Jasper Rebecca Sebastian Anna Harvey Holly Reuben Elise Rory Martha Stanley Beatrice Louis
Willow Archie Iris Mason Penelope Frederick Georgia Elijah Ada Kieran Nina Marcus Tessa Patrick Heidi Dominic Lydia Simon Julia Connor
Rachel Adrian Paige Vincent'''.split()
_LAST = '''Harrison Bennett Hughes Walsh Ellis Turner Gray Brooks Price Morgan Reid Kelly Ward Cooper Hayes Murray Barker Russell Sutton
Holmes Fletcher Chapman Webb Lawson Doyle Spencer Marsh Fraser Lambert Wood Ashworth Booth Carver Dalton Everett Farrow Gibbs Hale Irving
Jennings Kerr Lowe Mercer Norris Osborne Pearce Quinn Rhodes Shaw Thornton Underwood Vaughan Whitfield Yates Abbott Bishop Coleman Drake
Elliott Fowler Garner Hartley Ingram Jarvis Knight Lyons Manning Nash Oakley Parsons Rowe Sharp Tucker Vance Wilkins Archer Blake Chambers
Dawson Emerson Finch Gordon Hammond Iqbal Joyce Kendall Lindsay Moss Nolan Ortega Palmer Ramsey Stone Tate Upton Vickers Warner York Banks
Carr Dunn Eaton Frost Grant Hurst'''.split()


def _build_users():
    users, i = [], 0
    for n, (slug, _name, kind, _mission) in enumerate(TEAMS):
        size = 4 if n % 2 == 0 else 3
        for position in range(size):
            first, last = _FIRST[i], _LAST[i]
            i += 1
            users.append(dict(username=f'{first}.{last}'.lower(), firstName=first, lastName=last,
                              email=f'{first}.{last}@{EMAIL_DOMAIN}'.lower(), team=slug, groups=[slug],
                              title=ROLE_TITLES[kind][position], position=position))
    users.append(dict(username='platform.admin', firstName='Platform', lastName='Administrator', email='platform.admin@' + EMAIL_DOMAIN,
                      team=None, groups=[ADMIN_GROUP], title='vMCP Platform Administrator (console admin)', position=0))
    users.append(dict(username='security.auditor', firstName='Security', lastName='Auditor', email='security.auditor@' + EMAIL_DOMAIN,
                      team=None, groups=[VIEWER_GROUP], title='Security Auditor (read-only console access)', position=0))
    return users


USERS = _build_users()


def team_members(slug):
    return [u for u in USERS if u['team'] == slug]


def staff(team, position):
    return team_members(team)[position]


# slug, display name, owner team, business purpose
PROJECTS = [
    ('retail-mobile-assistant', 'Retail Mobile Banking Assistant', 'retail-digital-banking', 'In-app conversational assistant for everyday banking.'),
    ('retail-payments-copilot', 'Retail Payments Copilot', 'retail-digital-banking', 'Guided domestic and international payments in the mobile app.'),
    ('contact-center-agent-assist', 'Contact Center Agent Assist', 'contact-center-ai', 'Real-time copilot for contact center colleagues.'),
    ('ivr-voice-banking', 'IVR Voice Banking', 'contact-center-ai', 'Voice self-service for balances, card freezes and payment status.'),
    ('card-servicing-automation', 'Card Servicing Automation', 'cards-issuing', 'Automated card controls and servicing journeys.'),
    ('dispute-intake-automation', 'Dispute Intake Automation', 'disputes-chargebacks', 'Structured card dispute intake and triage.'),
    ('fraud-investigation-workbench', 'Fraud Investigation Workbench', 'fraud-strategy', 'Investigator copilot for open fraud alerts.'),
    ('realtime-fraud-triage', 'Real-time Fraud Triage', 'fraud-strategy', 'Autonomous first-line triage of high-risk card alerts.'),
    ('aml-alert-triage', 'AML Alert Triage', 'financial-crime-compliance', 'Investigation support for AML monitoring alerts.'),
    ('sanctions-screening-assistant', 'Sanctions Screening Assistant', 'financial-crime-compliance', 'Party screening and payment hold review.'),
    ('kyc-periodic-review', 'KYC Periodic Review', 'kyc-operations', 'Automated preparation of periodic and trigger-event reviews.'),
    ('digital-onboarding', 'Digital Customer Onboarding', 'kyc-operations', 'Onboarding journey: document checklist, screening and product fit.'),
    ('credit-decisioning', 'Credit Decisioning', 'credit-risk-analytics', 'Underwriting copilot combining bureau and internal data.'),
    ('personal-loan-journey', 'Personal Loan Digital Journey', 'lending-origination', 'End-to-end digital personal loan quote and application.'),
    ('mortgage-advisor', 'Mortgage Advisor Assistant', 'lending-origination', 'Mortgage product research and appointment booking.'),
    ('loan-servicing-desk', 'Loan Servicing Desk', 'lending-servicing', 'Servicing queries on live loans and settlements.'),
    ('collections-outreach', 'Collections Outreach', 'collections-recovery', 'Empathetic arrears outreach and payment arrangements.'),
    ('treasury-dealer-assistant', 'Treasury Dealer Assistant', 'treasury-markets', 'Market data and liquidity insight for the dealing desk.'),
    ('trade-finance-desk', 'Trade Finance Desk Copilot', 'trade-finance', 'Letter of credit and guarantee servicing for trade officers.'),
    ('wealth-advisor-copilot', 'Wealth Advisor Copilot', 'wealth-management', 'Portfolio reviews and suitability preparation.'),
    ('private-banker-briefing', 'Private Banker Client Briefing', 'private-banking', 'Pre-meeting client briefings for private bankers.'),
    ('sme-relationship-manager', 'SME Relationship Manager Assistant', 'sme-banking', 'Relationship overview and lending for SME clients.'),
    ('sme-cashflow-insights', 'SME Cash-flow Insights', 'sme-banking', 'Cash-flow analysis from business account activity.'),
    ('corporate-treasury-portal', 'Corporate Treasury Portal Assistant', 'corporate-banking', 'Liquidity, FX and cross-border payments for corporates.'),
    ('corporate-trade-services', 'Corporate Trade Services', 'corporate-banking', 'Trade instrument visibility for corporate clients.'),
    ('branch-teller-assistant', 'Branch Teller Assistant', 'branch-network', 'Counter and branch colleague copilot.'),
    ('complaints-resolution', 'Complaints Resolution Assistant', 'customer-care', 'Complaint intake, investigation and customer updates.'),
    ('customer-360', 'Customer 360 Insights', 'customer-data-management', 'Single customer view across products.'),
    ('core-ledger-operations', 'Core Ledger Operations', 'core-banking-platform', 'Ledger reconciliation and statement support.'),
    ('payment-operations-center', 'Payment Operations Center', 'payments-engineering', 'Payment exception handling and investigations.'),
    ('cross-border-payment-tracker', 'Cross-border Payment Tracker', 'cross-border-payments', 'SWIFT gpi tracking and FX enquiry handling.'),
    ('product-pricing-studio', 'Product Pricing Studio', 'product-pricing', 'Rate and fee benchmarking for product managers.'),
    ('customer-notification-orchestrator', 'Customer Notification Orchestrator', 'digital-engagement', 'Consent-aware outbound messaging.'),
    ('operational-risk-monitoring', 'Operational Risk Monitoring', 'operational-risk', 'Key risk indicator monitoring across operations.'),
    ('regulatory-reporting-assistant', 'Regulatory Reporting Assistant', 'regulatory-reporting', 'Data gathering for prudential and conduct returns.'),
    ('audit-evidence-collector', 'Audit Evidence Collector', 'internal-audit', 'Read-only evidence collection for audit testing.'),
    ('genai-sandbox', 'GenAI Sandbox', 'data-science-ai-lab', 'Low-risk experimentation with public product information.'),
    ('next-best-action', 'Next Best Action Engine', 'data-science-ai-lab', 'Personalised product and service recommendations.'),
    ('open-banking-aisp', 'Open Banking Account Information', 'open-banking-apis', 'Consent-based account information for third-party providers.'),
    ('open-banking-pisp', 'Open Banking Payment Initiation', 'open-banking-apis', 'Consent-based payment initiation for third-party providers.'),
]

# Each server: slug, display name, project, description, agent instructions, tools,
# and optional cross-team grant decisions left for the demo ('pending' / 'rejected').
SERVERS = [
    dict(slug='retail-banking-assistant', name='Retail Banking Assistant', project='retail-mobile-assistant',
         description='Everyday banking for authenticated retail customers: balances, transactions, card controls, payees, branches and products.',
         instructions='You assist an authenticated retail customer inside the Meridian Bank app. Only access data for the customer ID supplied by the session. Always confirm with the customer before freezing or unfreezing a card. Never reveal full card numbers. Suggest a branch appointment for mortgage or complex advice.',
         tools=['list_customer_accounts', 'get_account_balance', 'list_account_transactions', 'list_customer_cards', 'freeze_card', 'unfreeze_card',
                'list_saved_payees', 'find_branches', 'find_atms', 'list_banking_products']),
    dict(slug='payments-copilot', name='Payments Copilot', project='retail-payments-copilot',
         description='Guided domestic (Faster Payments, BACS, CHAPS) and international (SWIFT) payments with payee checks and FX quotes.',
         instructions='Before any payment: check the available balance, run validate_payee_name for new payees and read the result back to the customer. Obtain explicit confirmation of amount, payee and reference before calling initiate_domestic_payment or initiate_international_payment. For international payments always fetch get_fx_quote first.',
         tools=['list_customer_accounts', 'get_account_balance', 'list_saved_payees', 'validate_payee_name', 'initiate_domestic_payment', 'get_payment_status',
                'list_standing_orders', 'cancel_standing_order', 'get_fx_quote', 'initiate_international_payment']),
    dict(slug='contact-center-agent-assist', name='Contact Center Agent Assist', project='contact-center-agent-assist',
         description='Read-mostly copilot for contact center colleagues: identify the caller, summarise products, alerts, complaints and recent contact.',
         instructions='You support a human contact center colleague, not the customer directly. Start with search_customers to identify the caller, then summarise only what is relevant to the call reason. Highlight open fraud alerts, complaints, KYC actions and vulnerability indicators first.',
         tools=['search_customers', 'get_customer_profile', 'list_customer_accounts', 'list_account_transactions', 'list_customer_cards', 'get_card_details',
                'list_fraud_alerts', 'list_customer_complaints', 'list_notification_history', 'get_kyc_status']),
    dict(slug='voice-banking-ivr', name='Voice Banking IVR', project='ivr-voice-banking',
         description='Voice self-service: balance, recent transactions, emergency card freeze, payment status and branch finder.',
         instructions='Responses are read aloud: keep them short, state amounts with currency, and never read more than three transactions at a time. Card freezes are allowed after caller verification by the IVR; unfreezing is not available by voice.',
         tools=['get_account_balance', 'list_account_transactions', 'freeze_card', 'get_payment_status', 'find_branches']),
    dict(slug='card-servicing', name='Card Servicing', project='card-servicing-automation',
         description='Card lifecycle and controls: card details, freeze/unfreeze, spending limits, card transactions and customer notifications.',
         instructions='Use freeze_card with reason LOST or STOLEN only when the customer says the card is gone - this permanently blocks the card and orders a replacement. Use CUSTOMER_REQUEST for a temporary freeze. After any control change send the CARD_FROZEN notification when appropriate.',
         tools=['list_customer_cards', 'get_card_details', 'freeze_card', 'unfreeze_card', 'update_card_spending_limits', 'list_card_transactions', 'send_customer_notification']),
    dict(slug='card-dispute-intake', name='Card Dispute Intake', project='dispute-intake-automation',
         description='Structured intake of card disputes: identify the transaction, check fraud risk, open the dispute and log complaints.',
         instructions='Identify the exact transaction with list_card_transactions before opening a dispute. If the customer does not recognise the transaction, check get_transaction_risk_score and route to fraud instead of a merchant dispute. Never open a second dispute for a transaction that already has a disputeReference.',
         tools=['list_card_transactions', 'open_card_dispute', 'get_dispute_case', 'get_transaction_risk_score', 'log_customer_complaint']),
    dict(slug='fraud-investigation', name='Fraud Investigation', project='fraud-investigation-workbench',
         description='Investigator workbench for fraud alerts: risk scores, card and account activity, customer context, card freeze and outcomes.',
         instructions='Work alert by alert. Gather evidence (risk score, card transactions, customer profile) before recommending an outcome. Only call resolve_fraud_alert with CONFIRMED_FRAUD after the customer has denied the transaction, and freeze the card in the same session.',
         tools=['list_fraud_alerts', 'get_transaction_risk_score', 'resolve_fraud_alert', 'list_card_transactions', 'get_customer_profile', 'list_account_transactions',
                'freeze_card', 'send_customer_notification']),
    dict(slug='fraud-triage', name='Real-time Fraud Triage', project='realtime-fraud-triage',
         description='First-line automated triage for high-risk alerts: score, protect the card and contact the customer.',
         instructions='For HIGH severity alerts with a risk score of 0.9 or above, freeze the card with reason SUSPECTED_FRAUD and send FRAUD_ALERT_VERIFY. Do not resolve alerts; resolution belongs to human investigators.',
         tools=['list_fraud_alerts', 'get_transaction_risk_score', 'freeze_card', 'send_customer_notification']),
    dict(slug='aml-alert-triage', name='AML Alert Triage', project='aml-alert-triage',
         description='AML investigation support: alerts, cases, party screening, transaction history, KYC status and SWIFT tracking.',
         instructions='You support a financial crime investigator. Never disclose to anyone that an investigation or SAR consideration exists (tipping-off). Summarise facts with transaction IDs and dates; do not make the SAR decision.',
         tools=['list_aml_alerts', 'get_aml_case', 'screen_party', 'get_customer_profile', 'list_account_transactions', 'get_kyc_status', 'track_swift_payment']),
    dict(slug='sanctions-screening', name='Sanctions Screening', project='sanctions-screening-assistant',
         description='Sanctions and PEP screening of parties and review of held cross-border payments.',
         instructions='Screen every counterparty name exactly as provided. A POTENTIAL_MATCH must be escalated to Financial Crime Compliance; never advise releasing a held payment.',
         tools=['screen_party', 'track_swift_payment', 'get_customer_profile']),
    dict(slug='kyc-periodic-review', name='KYC Periodic Review', project='kyc-periodic-review',
         description='Prepares KYC periodic and trigger-event reviews: status, required documents, screening, refresh cases and document requests.',
         instructions='Check get_kyc_status first. If documents are expired or missing, start a refresh with start_kyc_refresh and send KYC_DOCUMENT_REQUEST through a channel the customer has consented to (check contact preferences via the profile).',
         tools=['get_kyc_status', 'list_required_kyc_documents', 'start_kyc_refresh', 'get_customer_profile', 'screen_party', 'send_customer_notification']),
    dict(slug='digital-onboarding', name='Digital Onboarding', project='digital-onboarding',
         description='New customer onboarding: document checklist, sanctions screening, product eligibility and credit check.',
         instructions='Screen the applicant with screen_party before recommending products. Only run get_credit_bureau_report for existing customer IDs with consent recorded by the journey.',
         tools=['list_required_kyc_documents', 'screen_party', 'list_banking_products', 'get_product_terms', 'get_credit_bureau_report']),
    dict(slug='credit-decisioning', name='Credit Decisioning', project='credit-decisioning',
         description='Underwriting copilot: bureau report, internal risk grade, existing exposure, account conduct and application status.',
         instructions='Summarise affordability and conduct evidence for a human underwriter. Cite bureau band, internal grade, arrears and policy flags. You do not approve or decline applications.',
         tools=['get_credit_bureau_report', 'get_internal_risk_grade', 'list_customer_loans', 'list_account_transactions', 'get_loan_application']),
    dict(slug='personal-loan-journey', name='Personal Loan Journey', project='personal-loan-journey',
         description='Digital personal loan journey: quote, product terms, risk pre-check, application submission and tracking.',
         instructions='Always present the representative APR and total cost from get_loan_quote before submitting. Submit only after the customer confirms amount, term and purpose.',
         tools=['get_loan_quote', 'submit_loan_application', 'get_loan_application', 'get_product_terms', 'get_internal_risk_grade']),
    dict(slug='mortgage-advisor', name='Mortgage Advisor', project='mortgage-advisor',
         description='Mortgage research: products, terms, indicative quotes, credit profile, rate curves and advisor appointments.',
         instructions='Provide information, not regulated advice. For a recommendation, book an appointment with a mortgage advisor using book_branch_appointment (topic MORTGAGE).',
         tools=['list_banking_products', 'get_product_terms', 'get_loan_quote', 'get_credit_bureau_report', 'get_interest_rate_curve', 'book_branch_appointment']),
    dict(slug='loan-servicing-desk', name='Loan Servicing Desk', project='loan-servicing-desk',
         description='Live loan servicing: loan details, repayment schedules and early settlement figures.',
         instructions='Settlement figures are valid only until the validUntil date returned. Mention early repayment charges explicitly.',
         tools=['list_customer_loans', 'get_loan_details', 'get_repayment_schedule', 'get_early_settlement_quote', 'get_customer_profile']),
    dict(slug='collections-outreach', name='Collections Outreach', project='collections-outreach',
         description='Arrears outreach: arrears list, collection cases, affordable payment arrangements and consent-aware reminders.',
         instructions='Follow fair treatment of vulnerable customers. Check vulnerability indicators and contact preferences before any contact. Propose arrangements the customer can afford; never threaten or pressure.',
         tools=['list_arrears_accounts', 'get_collection_case', 'propose_payment_arrangement', 'get_repayment_schedule', 'send_customer_notification',
                'get_customer_contact_preferences']),
    dict(slug='treasury-dealer-desk', name='Treasury Dealer Desk', project='treasury-dealer-assistant',
         description='Market data and liquidity for the treasury desk: FX rates, rate curves, liquidity position and FX payment quotes.',
         instructions='Quote rates with their timestamp. Flag any liquidity alert or a currency position above 60% limit utilisation.',
         tools=['get_fx_rates', 'get_interest_rate_curve', 'get_liquidity_position', 'get_fx_quote']),
    dict(slug='trade-finance-desk', name='Trade Finance Desk', project='trade-finance-desk',
         description='Trade officer copilot: letters of credit, discrepancies, guarantees, party screening and FX rates.',
         instructions='Screen all trade parties (applicant, beneficiary, banks) with screen_party when reviewing a new presentation. List discrepancies verbatim.',
         tools=['list_letters_of_credit', 'get_letter_of_credit', 'list_bank_guarantees', 'screen_party', 'get_fx_rates']),
    dict(slug='wealth-advisor', name='Wealth Advisor', project='wealth-advisor-copilot',
         description='Portfolio review preparation: valuation, holdings, investor risk profile and FX exposure.',
         instructions='Compare the portfolio risk level with the investor risk profile and flag mismatches or overdue suitability reviews. Do not recommend specific securities.',
         tools=['get_portfolio_summary', 'list_portfolio_holdings', 'get_investor_risk_profile', 'get_customer_profile', 'get_fx_rates']),
    dict(slug='private-banker-briefing', name='Private Banker Briefing', project='private-banker-briefing',
         description='One-page client briefing for private bankers: relationship, balances, portfolio, lending, cards and open complaints.',
         instructions='Produce a concise briefing with sections: relationship, liquidity, investments, credit, service issues, suggested talking points.',
         tools=['get_customer_profile', 'list_customer_accounts', 'get_portfolio_summary', 'list_customer_loans', 'list_customer_cards', 'list_customer_complaints',
                'book_branch_appointment']),
    dict(slug='sme-relationship-manager', name='SME Relationship Manager', project='sme-relationship-manager',
         description='SME relationship overview and lending: profile, accounts, loans, risk grade, applications and products.',
         instructions='Check get_internal_risk_grade policy flags before discussing new lending. Do not submit applications for customers with open AML investigations; refer them to Financial Crime Compliance.',
         tools=['get_customer_profile', 'list_customer_accounts', 'list_customer_loans', 'get_internal_risk_grade', 'submit_loan_application', 'get_loan_application',
                'list_banking_products']),
    dict(slug='sme-cashflow-insights', name='SME Cash-flow Insights', project='sme-cashflow-insights',
         description='Cash-flow insight for SME clients from balances, transactions, statements and standing orders.',
         instructions='Group transactions by category, identify the largest inflows and outflows and upcoming standing orders. Present figures in the account currency.',
         tools=['list_customer_accounts', 'list_account_transactions', 'get_account_statement', 'list_standing_orders', 'get_account_balance']),
    dict(slug='corporate-treasury', name='Corporate Treasury', project='corporate-treasury-portal',
         description='Corporate cash management: multi-currency balances, FX quotes and rates, SWIFT payments and gpi tracking.',
         instructions='Corporate payments above 1,000,000 in any currency require dual authorisation outside this assistant: prepare the details and do not submit them.',
         tools=['list_customer_accounts', 'get_account_balance', 'get_fx_quote', 'initiate_international_payment', 'track_swift_payment', 'get_fx_rates']),
    dict(slug='corporate-trade-services', name='Corporate Trade Services', project='corporate-trade-services',
         description='Trade instrument visibility for corporate clients: letters of credit, guarantees and settlement tracking.',
         instructions='Explain discrepancies in plain language and what documents the client must correct.',
         tools=['list_letters_of_credit', 'get_letter_of_credit', 'list_bank_guarantees', 'track_swift_payment']),
    dict(slug='branch-teller-assistant', name='Branch Teller Assistant', project='branch-teller-assistant',
         description='Branch colleague copilot: customer lookup, balances, transactions, contact detail updates, KYC and appointments.',
         instructions='Customer identity is verified in branch by the colleague. Contact detail changes trigger verification to the existing mobile number; tell the customer to expect a passcode.',
         tools=['search_customers', 'get_customer_profile', 'get_account_balance', 'list_account_transactions', 'update_customer_contact_details',
                'book_branch_appointment', 'get_kyc_status']),
    dict(slug='complaints-resolution', name='Complaints Resolution', project='complaints-resolution',
         description='Complaint handling: history, details, new complaints, related disputes and customer updates.',
         instructions='Log every expression of dissatisfaction as a complaint. Quote the final response deadline. Use COMPLAINT_ACKNOWLEDGEMENT after logging.',
         tools=['list_customer_complaints', 'get_complaint', 'log_customer_complaint', 'get_dispute_case', 'list_notification_history', 'send_customer_notification']),
    dict(slug='customer-360', name='Customer 360', project='customer-360',
         description='Single customer view: profile, preferences, accounts, cards, loans and investments.',
         instructions='Present a structured customer view. Mask nothing beyond what services return; never infer data that is not returned.',
         tools=['search_customers', 'get_customer_profile', 'get_customer_contact_preferences', 'list_customer_accounts', 'list_customer_cards', 'list_customer_loans',
                'get_portfolio_summary']),
    dict(slug='core-ledger-operations', name='Core Ledger Operations', project='core-ledger-operations',
         description='Ledger operations: account master data, balances, holds, transactions and statements.',
         instructions='Reconcile ledger and available balances by explaining holds and overdraft limits. Reference transaction IDs.',
         tools=['get_account_details', 'get_account_balance', 'list_account_transactions', 'get_account_statement']),
    dict(slug='payment-operations', name='Payment Operations', project='payment-operations-center',
         description='Payment exception handling: payment status, standing orders, SWIFT tracking and transaction risk.',
         instructions='For rejected payments, state the rejection reason and the recommended next action. Cancel standing orders only on documented customer instruction.',
         tools=['get_payment_status', 'list_standing_orders', 'cancel_standing_order', 'track_swift_payment', 'get_transaction_risk_score']),
    dict(slug='cross-border-tracker', name='Cross-border Payment Tracker', project='cross-border-payment-tracker',
         description='SWIFT gpi tracking, FX quotes and rates, and counterparty screening for cross-border enquiries.',
         instructions='Explain gpi status codes (ACSP in progress, ACCC credited, PDNG/ON_HOLD pending) in plain language. Do not disclose compliance hold reasons to customers.',
         tools=['track_swift_payment', 'get_fx_quote', 'get_fx_rates', 'screen_party']),
    dict(slug='product-pricing-studio', name='Product Pricing Studio', project='product-pricing-studio',
         description='Product catalogue, terms, savings rates and market rate curves for pricing analysis.',
         instructions='Compare product rates against the relevant SONIA tenor and report the margin in basis points.',
         tools=['list_banking_products', 'get_product_terms', 'get_savings_rates', 'get_interest_rate_curve']),
    dict(slug='notification-orchestrator', name='Notification Orchestrator', project='customer-notification-orchestrator',
         description='Consent-aware outbound messaging: templates, preferences, sending and history.',
         instructions='Check get_customer_contact_preferences before sending and use only consented channels. Supply every template parameter listed by list_notification_templates.',
         tools=['send_customer_notification', 'list_notification_history', 'list_notification_templates', 'get_customer_contact_preferences']),
    dict(slug='operational-risk-monitor', name='Operational Risk Monitor', project='operational-risk-monitoring',
         description='Key risk indicators: liquidity position, arrears, AML alerts and complaint details.',
         instructions='Report indicators as a RAG (red/amber/green) summary with the underlying figures.',
         tools=['get_liquidity_position', 'list_arrears_accounts', 'list_aml_alerts', 'get_complaint']),
    dict(slug='regulatory-reporting', name='Regulatory Reporting', project='regulatory-reporting-assistant',
         description='Data gathering for prudential and conduct returns: liquidity ratios, rate curves, arrears and AML alert volumes.',
         instructions='Return figures with their as-of timestamps and source tool so they can be traced in the return working papers.',
         tools=['get_liquidity_position', 'get_interest_rate_curve', 'list_arrears_accounts', 'list_aml_alerts']),
    dict(slug='audit-evidence', name='Audit Evidence Collector', project='audit-evidence-collector',
         description='Read-only evidence collection for audit testing across AML, KYC, complaints, disputes and statements.',
         instructions='Read-only use. Record every reference you retrieve as evidence with its retrieval time.',
         tools=['get_aml_case', 'get_kyc_status', 'get_complaint', 'get_dispute_case', 'get_account_statement']),
    dict(slug='genai-sandbox', name='GenAI Sandbox', project='genai-sandbox',
         description='Experimentation server with public product information, branch data and FX rates. Payment initiation is awaiting approval and sanctions screening was rejected by the owning teams.',
         instructions='Sandbox use only. Do not use real customer data.',
         tools=['list_banking_products', 'get_product_terms', 'find_branches', 'find_atms', 'get_fx_rates', 'initiate_domestic_payment', 'screen_party'],
         grants={'initiate_domestic_payment': 'pending', 'screen_party': 'rejected'},
         grantReasons={'screen_party': 'Sanctions screening is restricted to Financial Crime Compliance approved use cases.'}),
    dict(slug='next-best-action', name='Next Best Action', project='next-best-action',
         description='Personalised recommendations from profile, product holdings, transaction patterns, risk profile and marketing consent.',
         instructions='Only recommend products when marketingOptIn is true. Never recommend credit to customers with vulnerability indicators or arrears.',
         tools=['get_customer_profile', 'list_customer_accounts', 'list_account_transactions', 'list_banking_products', 'get_investor_risk_profile',
                'get_customer_contact_preferences']),
    dict(slug='open-banking-aisp', name='Open Banking AISP', project='open-banking-aisp',
         description='Account information for consented third-party providers: accounts, details, balances and transactions.',
         instructions='Return only accounts covered by the third-party consent. Data is read-only.',
         tools=['list_customer_accounts', 'get_account_details', 'get_account_balance', 'list_account_transactions']),
    dict(slug='open-banking-pisp', name='Open Banking PISP', project='open-banking-pisp',
         description='Payment initiation for consented third-party providers with Confirmation of Payee.',
         instructions='Run validate_payee_name before every payment and stop on NO_MATCH. Payments are limited to the consented source account.',
         tools=['validate_payee_name', 'initiate_domestic_payment', 'get_payment_status']),
]

# Runtime keys for agent personas (one-time secrets stored in the quickstart Secret).
AGENT_KEYS = [
    ('retail-channels-agent', 'Retail app, voice and open banking agents', ['retail-banking-assistant', 'payments-copilot', 'voice-banking-ivr', 'open-banking-aisp', 'open-banking-pisp']),
    ('contact-center-agent', 'Contact center and servicing agents', ['contact-center-agent-assist', 'card-servicing', 'card-dispute-intake', 'complaints-resolution', 'branch-teller-assistant']),
    ('fraud-ops-agent', 'Fraud triage and investigation agents', ['fraud-investigation', 'fraud-triage']),
    ('financial-crime-agent', 'AML, sanctions and KYC agents', ['aml-alert-triage', 'sanctions-screening', 'kyc-periodic-review', 'digital-onboarding']),
    ('lending-agent', 'Credit, origination, servicing and collections agents', ['credit-decisioning', 'personal-loan-journey', 'mortgage-advisor', 'loan-servicing-desk', 'collections-outreach']),
    ('relationship-manager-agent', 'Wealth, private, SME and customer insight agents', ['wealth-advisor', 'private-banker-briefing', 'sme-relationship-manager', 'sme-cashflow-insights', 'customer-360', 'next-best-action']),
    ('corporate-markets-agent', 'Treasury, trade and corporate banking agents', ['treasury-dealer-desk', 'trade-finance-desk', 'corporate-treasury', 'corporate-trade-services', 'cross-border-tracker']),
    ('operations-agent', 'Ledger, payment, notification and pricing operations agents', ['core-ledger-operations', 'payment-operations', 'notification-orchestrator', 'product-pricing-studio']),
    ('risk-compliance-agent', 'Operational risk, regulatory reporting and audit agents', ['operational-risk-monitor', 'regulatory-reporting', 'audit-evidence']),
    ('sandbox-agent', 'GenAI sandbox experiments', ['genai-sandbox']),
]

TEAM = {t[0]: dict(slug=t[0], name=t[1], kind=t[2], mission=t[3]) for t in TEAMS}
PROJECT = {p[0]: dict(slug=p[0], name=p[1], ownerTeam=p[2], purpose=p[3]) for p in PROJECTS}
SERVER = {s['slug']: s for s in SERVERS}


def server_team(server):
    return PROJECT[server['project']]['ownerTeam']


def team_servers(team):
    return [s['slug'] for s in SERVERS if server_team(s) == team]


def server_keys(server_slug):
    return [k for k, _d, servers in AGENT_KEYS if server_slug in servers]


def identity_binding_slug(team):
    return team + '-sso'
